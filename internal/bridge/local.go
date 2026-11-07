package bridge

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

type localModel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Repo       string `json:"-"`
	Revision   string `json:"-"`
	Bytes      int64  `json:"bytes"`
	MinimumRAM int64  `json:"-"`
}

var localModels = []localModel{
	{"qwen3-4b", "Qwen3 4B", "mlx-community/Qwen3-4B-Instruct-2507-4bit", "50d427756c6b1b2fe0c0a10f67fbda1fc8e82c1b", 2500000000, 8 << 30},
	{"qwen3-8b", "Qwen3 8B", "mlx-community/Qwen3-8B-4bit", "545dc4251c05440727734bcd94334791f6ab0192", 5000000000, 16 << 30},
	{"qwen3-coder-30b", "Qwen3-Coder 30B", "mlx-community/Qwen3-Coder-30B-A3B-Instruct-4bit", "6e302ea604ad9ab206367e2c501d1571023e7b6d", 18000000000, 32 << 30},
}

func findLocalModel(id string) (localModel, bool) {
	for _, m := range localModels {
		if m.ID == id {
			return m, true
		}
	}
	return localModel{}, false
}
func localModelFits(total, available int64, model localModel) bool {
	return total >= model.MinimumRAM && available >= model.Bytes+(4<<30)
}
func recommendedLocalModel(total, available int64) string {
	for _, id := range []string{"qwen3-coder-30b", "qwen3-4b"} {
		model, _ := findLocalModel(id)
		if localModelFits(total, available, model) {
			return id
		}
	}
	return ""
}

type localSetup struct {
	Model string `json:"model"`
	Stage string `json:"stage"`
	Error string `json:"error,omitempty"`
	Done  int64  `json:"downloaded"`
	Total int64  `json:"total"`
}
type localManager struct {
	app                            *App
	mu                             sync.Mutex
	setup                          localSetup
	cancel                         context.CancelFunc
	running                        bool
	runtime                        *exec.Cmd
	runtimeCancel                  context.CancelFunc
	runtimeDone                    chan struct{}
	runtimeModel, endpoint, secret string
	client                         *http.Client
}

func newLocalManager(a *App) *localManager {
	return &localManager{app: a, client: &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (l *localManager) path(parts ...string) string {
	return filepath.Join(append([]string{l.app.path("local")}, parts...)...)
}
func (l *localManager) status() localSetup { l.mu.Lock(); defer l.mu.Unlock(); return l.setup }
func (l *localManager) progress(stage string, done, total int64) {
	l.mu.Lock()
	l.setup.Stage, l.setup.Done, l.setup.Total = stage, done, total
	l.mu.Unlock()
}
func (l *localManager) hardware() (int64, int64, error) {
	total, available, err := localHardware()
	l.mu.Lock()
	id := l.runtimeModel
	l.mu.Unlock()
	if model, ok := findLocalModel(id); ok {
		available = min(total, available+model.Bytes)
	}
	return total, available, err
}
func (l *localManager) catalog() object {
	total, available, err := l.hardware()
	recommendation := recommendedLocalModel(total, available)
	entries := []object{}
	saved := l.app.snapshot().LocalModels
	for _, m := range localModels {
		reason := ""
		if err != nil {
			reason = err.Error()
		} else if !localModelFits(total, available, m) {
			reason = "Not enough available memory"
		}
		entries = append(entries, object{"id": m.ID, "name": m.Name, "bytes": m.Bytes, "recommended": m.ID == recommendation, "available": reason == "", "reason": reason, "added": slices.Contains(saved, m.ID)})
	}
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return object{"models": entries, "recommended": recommendation, "supported": err == nil, "error": reason, "ram": total, "availableRAM": available}
}
func (l *localManager) add(id string) error {
	model, ok := findLocalModel(id)
	if !ok {
		return errors.New("Choose a local model")
	}
	total, available, err := l.hardware()
	if err != nil {
		return err
	}
	if !localModelFits(total, available, model) {
		return errors.New("Not enough available memory for this model")
	}
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return errors.New("A local model is already being prepared")
	}
	ctx, cancel := context.WithCancel(l.app.ctx)
	l.cancel = cancel
	l.running = true
	l.setup = localSetup{Model: id, Stage: "Preparing"}
	l.mu.Unlock()
	l.app.wg.Add(1)
	go func() {
		defer l.app.wg.Done()
		defer cancel()
		err := l.prepare(ctx, model)
		// Preparing the first local source must not stall live GitHub inference.
		if err == nil && l.app.snapshot().LocalModel == "" {
			err = l.startRuntime(ctx, model.ID)
			if err == nil {
				err = l.checkModel(ctx, model.ID)
			}
		}
		if err == nil {
			l.progress("Connecting", 0, 0)
			for {
				l.app.actions.Lock()
				err = l.activate(ctx, model.ID)
				l.app.actions.Unlock()
				if !errors.Is(err, errLocalBusy) {
					break
				}
				select {
				case <-ctx.Done():
					err = ctx.Err()
				case <-time.After(time.Second):
					continue
				}
				break
			}
		}
		if err != nil && l.app.snapshot().LocalModel == "" {
			l.stopRuntime()
		}
		l.mu.Lock()
		l.running = false
		l.cancel = nil
		if errors.Is(err, context.Canceled) {
			l.setup.Stage = "Paused"
		} else if err != nil {
			l.setup.Stage = "Error"
			l.setup.Error = err.Error()
		} else {
			l.setup.Stage = "Ready"
		}
		l.mu.Unlock()
	}()
	return nil
}
func (l *localManager) cancelSetup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cancel != nil {
		l.cancel()
	}
}
func (l *localManager) close() { l.cancelSetup(); l.stopRuntime() }
func (l *localManager) stopRuntime() {
	l.mu.Lock()
	cancel, done := l.runtimeCancel, l.runtimeDone
	l.runtime = nil
	l.runtimeCancel = nil
	l.runtimeDone = nil
	l.runtimeModel = ""
	l.endpoint = ""
	l.secret = ""
	l.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

var errLocalBusy = errors.New("Waiting for the current request to finish")

func (l *localManager) activate(ctx context.Context, id string) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !l.app.routes.TryLock() {
		return errLocalBusy
	}
	defer l.app.routes.Unlock()
	previous := l.app.snapshot()
	changed := false
	defer func() {
		if result == nil {
			return
		}
		if changed {
			l.app.mu.Lock()
			l.app.state = previous
			l.app.bearer = ""
			l.app.expiry = 0
			l.app.rawModels = nil
			l.app.accountGeneration++
			l.app.mu.Unlock()
			l.app.models.setAccount(previous.Token)
			if previous.LocalModel != "" {
				result = errors.Join(result, l.startRuntime(l.app.ctx, previous.LocalModel), l.writeCatalog(previous.LocalModel))
			}
			if previous.Codex && previous.active() {
				result = errors.Join(result, l.app.injectCodex())
			} else {
				result = errors.Join(result, l.app.restoreCodex())
			}
			if previous.Claude && previous.active() {
				result = errors.Join(result, l.app.injectClaude())
			} else {
				result = errors.Join(result, l.app.restoreClaude())
			}
			result = errors.Join(result, l.app.saveSession())
		}
		l.app.copilotRouting = copilotRouting{}
	}()
	if previous.Copilot {
		if err := l.app.restoreCopilot(); err != nil {
			if errors.Is(err, errCopilotSessionBusy) {
				return errLocalBusy
			}
			return fmt.Errorf("Wait for Copilot to become idle: %w", err)
		}
	}
	if err := l.startRuntime(ctx, id); err != nil {
		if previous.LocalModel != "" {
			_ = l.startRuntime(l.app.ctx, previous.LocalModel)
		}
		return err
	}
	if err := l.checkModel(ctx, id); err != nil {
		if previous.LocalModel != "" && previous.LocalModel != id {
			_ = l.startRuntime(l.app.ctx, previous.LocalModel)
		}
		return err
	}
	next := previous
	next.Accounts = savedAccounts(previous)
	next.Token = ""
	next.Username = localModelName(id)
	next.Method = "local"
	next.LocalModel = id
	if !slices.Contains(next.LocalModels, id) {
		next.LocalModels = append(append([]string(nil), next.LocalModels...), id)
	}
	if !previous.active() {
		next.Codex = true
		next.Claude = true
	}
	l.app.models.pause()
	changed = true
	l.app.mu.Lock()
	l.app.state = next
	l.app.bearer = ""
	l.app.expiry = 0
	l.app.rawModels = nil
	l.app.rawAt = time.Time{}
	l.app.usage = nil
	l.app.lastEffort = nil
	l.app.accountGeneration++
	l.app.mu.Unlock()
	l.app.models.setAccount("")
	l.app.copilotRouting = copilotRouting{}
	if err := l.writeCatalog(id); err != nil {
		return err
	}
	if next.Codex {
		if err := l.app.injectCodex(); err != nil {
			return err
		}
	}
	if next.Claude {
		if err := l.app.injectClaude(); err != nil {
			return err
		}
	}
	if err := l.app.saveSession(); err != nil {
		return err
	}
	if next.Copilot {
		_, err := l.app.enableCopilot()
		return err
	}
	return nil
}
func localModelName(id string) string { m, _ := findLocalModel(id); return m.Name }
func (l *localManager) switchModel(ctx context.Context, id string) (object, error) {
	if !slices.Contains(l.app.snapshot().LocalModels, id) {
		return nil, errors.New("Local model account not found")
	}
	// Setup and switching are asynchronous: model load can outlive the UI request.
	if err := l.add(id); err != nil {
		return nil, err
	}
	return object{"ok": true, "preparing": true}, nil
}
func (l *localManager) deleteModel(id string) error {
	if l.status().Model == id {
		l.cancelSetup()
	}
	s := l.app.snapshot()
	if !slices.Contains(s.LocalModels, id) {
		return errors.New("Local model account not found")
	}
	if s.LocalModel == id {
		if err := l.app.disconnect(); err != nil {
			return err
		}
		s = l.app.snapshot()
	}
	s.LocalModels = slices.DeleteFunc(append([]string(nil), s.LocalModels...), func(value string) bool { return value == id })
	if err := writeJSON(l.app.path("session.json"), s); err != nil {
		return err
	}
	l.app.mu.Lock()
	l.app.state = s
	l.app.mu.Unlock()
	return nil
}
func (l *localManager) restore(id string) error {
	if _, _, err := localHardware(); err != nil {
		return err
	}
	if _, ok := findLocalModel(id); !ok {
		return errors.New("Saved local model is unavailable")
	}
	if err := l.startRuntime(l.app.ctx, id); err != nil {
		return err
	}
	if err := l.checkModel(l.app.ctx, id); err != nil {
		l.stopRuntime()
		return err
	}
	l.mu.Lock()
	l.setup = localSetup{Model: id, Stage: "Ready"}
	l.mu.Unlock()
	return l.writeCatalog(id)
}
func (l *localManager) contextSize(id string) int {
	total, _, _ := localHardware()
	if id == "qwen3-coder-30b" && total < 48<<30 {
		return 16384
	}
	return 32768
}
func (l *localManager) models(id string) []object {
	m, ok := findLocalModel(id)
	if !ok {
		return nil
	}
	return []object{{"id": id, "name": m.Name, "vendor": "Local", "model_picker_enabled": true, "supported_endpoints": []any{"/responses", "/chat/completions"}, "capabilities": object{"type": "chat", "limits": object{"max_context_window_tokens": l.contextSize(id), "max_prompt_tokens": l.contextSize(id), "max_output_tokens": 4096}, "supports": object{"streaming": true, "tool_calls": true}}}}
}
func (l *localManager) writeCatalog(id string) error {
	m := fallbackModel(id, localModelName(id), 0)
	m["description"] = "Local · oMLX"
	m["context_window"] = l.contextSize(id)
	m["max_context_window"] = l.contextSize(id)
	m["input_modalities"] = []any{"text"}
	m["supports_image_detail_original"] = false
	m["supports_search_tool"] = false
	m["support_verbosity"] = false
	m["apply_patch_tool_type"] = "function"
	m["default_reasoning_summary"] = "none"
	m["use_responses_lite"] = false
	return writeJSON(l.app.path("codex-model-catalog.json"), object{"models": []any{m}})
}
func (l *localManager) runtimeConfig(id string, port int, secret string) error {
	base := l.path("server", id)
	total, _, _ := localHardware()
	tier := "balanced"
	// oMLX's balanced reserve is 3 GiB; its small-Mac tier still enforces
	// both a dynamic memory ceiling and Apple's Metal working-set cap.
	if total > 0 && total <= 16<<30 {
		tier = "aggressive"
	}
	if err := os.MkdirAll(filepath.Join(base, "models"), 0700); err != nil {
		return err
	}
	link := filepath.Join(base, "models", id)
	if _, err := os.Lstat(link); errors.Is(err, os.ErrNotExist) {
		if err = os.Symlink(l.path("models", id), link); err != nil {
			return err
		}
	}
	if err := writeJSON(filepath.Join(base, "settings.json"), object{
		"server":    object{"host": "127.0.0.1", "port": port, "log_level": "warning"},
		"auth":      object{"api_key": secret, "skip_api_key_verification": false},
		"model":     object{"model_dirs": []any{filepath.Join(base, "models")}, "model_fallback": false},
		"scheduler": object{"max_concurrent_requests": 1},
		"memory":    object{"memory_guard_tier": tier, "prefill_memory_guard": true},
		"cache":     object{"ssd_cache_max_size": "2GB"},
	}); err != nil {
		return err
	}
	return writeJSON(filepath.Join(base, "model_settings.json"), object{"version": 1, "models": object{id: object{
		"is_default": true, "is_pinned": true, "enable_thinking": false, "max_context_window": l.contextSize(id), "max_tokens": 4096,
		"turboquant_kv_enabled": true, "turboquant_kv_scheme": "turboquant", "turboquant_kv_bits": 4, "turboquant_skip_last": true,
	}}})
}
func localSecret() string { return rand.Text() }
