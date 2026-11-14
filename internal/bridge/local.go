package bridge

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

type localModel struct {
	File       string `json:"-"`
	SHA256     string `json:"-"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Repo       string `json:"-"`
	Revision   string `json:"-"`
	Bytes      int64  `json:"bytes"`
	MinimumRAM int64  `json:"-"`
	MaxContext int    `json:"-"`
}

var localModels = []localModel{
	{ID: "qwen3-4b", Name: "Qwen3 4B", Repo: "lmstudio-community/Qwen3-4B-Instruct-2507-GGUF", Revision: "4edb920b6f14e3b9284d4502a6485103d72cde05", Bytes: 2497280448, MinimumRAM: 8 << 30, MaxContext: 262144, File: "Qwen3-4B-Instruct-2507-Q4_K_M.gguf", SHA256: "8cdb57cbb880d313736a9bc4e3d3d2485f145b5e19cf33783746e753e82641fc"},
	{ID: "qwen3-8b", Name: "Qwen3 8B", Repo: "Qwen/Qwen3-8B-GGUF", Revision: "7c41481f57cb95916b40956ab2f0b139b296d974", Bytes: 5027783488, MinimumRAM: 16 << 30, MaxContext: 40960, File: "Qwen3-8B-Q4_K_M.gguf", SHA256: "d98cdcbd03e17ce47681435b5150e34c1417f50b5c0019dd560e4882c5745785"},
	{ID: "qwen3-coder-30b", Name: "Qwen3-Coder 30B", Repo: "unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF", Revision: "b17cb02dd882d5b6ab62fc777ad2995f19668350", Bytes: 18556689568, MinimumRAM: 32 << 30, MaxContext: 262144, File: "Qwen3-Coder-30B-A3B-Instruct-Q4_K_M.gguf", SHA256: "fadc3e5f8d42bf7e894a785b05082e47daee4df26680389817e2093056f088ad"},
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
func localModelInstallReason(total int64, model localModel) string {
	if total < model.MinimumRAM {
		return fmt.Sprintf("Requires at least %d GB of memory", model.MinimumRAM>>30)
	}
	return ""
}
func recommendedLocalModel(total, available int64) string {
	for _, id := range []string{"qwen3-coder-30b", "qwen3-8b", "qwen3-4b"} {
		model, _ := findLocalModel(id)
		if localModelFits(total, available, model) {
			return id
		}
	}
	return ""
}

type localSetup struct {
	Model          string `json:"model"`
	Stage          string `json:"stage"`
	Error          string `json:"error,omitempty"`
	Done           int64  `json:"downloaded"`
	Total          int64  `json:"total"`
	CompletedSteps int    `json:"completedSteps"`
	TotalSteps     int    `json:"totalSteps"`
	Detail         string `json:"detail,omitempty"`
	Packages       int    `json:"packages"`
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
	runtimeContext                 int
	contexts                       map[string]int
	client                         *http.Client
}

func newLocalManager(a *App) *localManager {
	return &localManager{app: a, contexts: map[string]int{}, client: &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func localContextChoices(id string) []int {
	model, ok := findLocalModel(id)
	if !ok {
		return nil
	}
	choices := []int{}
	for _, value := range []int{8192, 16384, 32768, 65536, 131072, 262144} {
		if value <= model.MaxContext {
			choices = append(choices, value)
		}
	}
	if !slices.Contains(choices, model.MaxContext) {
		choices = append(choices, model.MaxContext)
	}
	return choices
}
func recommendedLocalContext(total int64, id string) int {
	switch id {
	case "qwen3-4b":
		switch {
		case total >= 64<<30:
			return 262144
		case total >= 32<<30:
			return 131072
		case total >= 24<<30:
			return 65536
		case total >= 16<<30:
			return 32768
		default:
			return 8192
		}
	case "qwen3-8b":
		if total >= 16<<30 {
			return 40960
		}
		return 8192
	case "qwen3-coder-30b":
		switch {
		case total >= 96<<30:
			return 262144
		case total >= 64<<30:
			return 131072
		case total >= 48<<30:
			return 65536
		default:
			return 16384
		}
	}
	return 0
}
func (l *localManager) loadContexts() error {
	data, err := readOptional(l.app.path("local-context.json"))
	if err != nil {
		return err
	}
	if data == nil {
		return nil
	}
	var saved map[string]int
	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("Invalid local context settings: %w", err)
	}
	for id, value := range saved {
		if !slices.Contains(localContextChoices(id), value) {
			return fmt.Errorf("Invalid saved context window for %s", id)
		}
	}
	l.mu.Lock()
	if saved != nil {
		l.contexts = saved
	}
	l.mu.Unlock()
	return nil
}
func (l *localManager) setContext(id string, value int) (object, error) {
	if !slices.Contains(localContextChoices(id), value) {
		return nil, errors.New("Choose a supported context window for this local model")
	}
	// llama.cpp allocates its KV cache at startup. Never replace a live engine.
	if !l.app.routes.TryLock() {
		return nil, errLocalBusy
	}
	defer l.app.routes.Unlock()
	l.mu.Lock()
	next := maps.Clone(l.contexts)
	next[id] = value
	active := l.runtimeModel == id && l.endpoint != "" && l.runtimeContext > 0
	previous := l.runtimeContext
	l.mu.Unlock()
	rollback := func(err error) (object, error) {
		if active && previous != value {
			if restore := l.startRuntimeAtContext(l.app.ctx, id, previous); restore != nil {
				err = errors.Join(err, fmt.Errorf("Restore previous local context: %w", restore))
			} else {
				l.mu.Lock()
				l.setup.Stage = "Ready"
				l.setup.Error = ""
				l.mu.Unlock()
			}
		}
		return nil, err
	}
	if active && previous != value {
		if err := l.startRuntimeAtContext(l.app.ctx, id, value); err != nil {
			return rollback(fmt.Errorf("Could not apply local context: %w", err))
		}
	}
	if err := writeJSON(l.app.path("local-context.json"), next); err != nil {
		return rollback(err)
	}
	l.mu.Lock()
	l.contexts = next
	if active {
		l.setup.Stage = "Ready"
		l.setup.Error = ""
	}
	l.mu.Unlock()
	if l.app.snapshot().LocalModel == id {
		if err := l.writeCatalog(id); err != nil {
			return nil, fmt.Errorf("Context applied, but could not update the Codex model catalog: %w", err)
		}
	}
	return object{"model": id, "contextWindow": value, "activeContextWindow": value, "contextPending": false}, nil
}
func (l *localManager) path(parts ...string) string {
	return filepath.Join(append([]string{l.app.path("local")}, parts...)...)
}
func (l *localManager) status() localSetup { l.mu.Lock(); defer l.mu.Unlock(); return l.setup }
func (l *localManager) progress(stage string, done, total int64) {
	l.mu.Lock()
	l.setup.Stage, l.setup.Done, l.setup.Total = stage, done, total
	if stage != "Installing" {
		l.setup.CompletedSteps, l.setup.TotalSteps, l.setup.Packages = 0, 0, 0
		l.setup.Detail = ""
	}
	l.mu.Unlock()
}
func (l *localManager) installProgress(completed int, detail string) {
	l.mu.Lock()
	l.setup.Stage, l.setup.Detail = "Installing", detail
	l.setup.CompletedSteps, l.setup.TotalSteps = completed, 6
	l.setup.Done, l.setup.Total, l.setup.Packages = 0, 0, 0
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
		} else {
			reason = localModelInstallReason(total, m)
		}
		warning := ""
		if reason == "" && !localModelFits(total, available, m) {
			warning = "Free memory before starting this model"
		}
		context, active := l.configuredContextSize(m.ID), l.contextSize(m.ID)
		entries = append(entries, object{"id": m.ID, "name": m.Name, "bytes": m.Bytes, "recommended": m.ID == recommendation, "available": reason == "", "reason": reason, "warning": warning, "added": slices.Contains(saved, m.ID),
			"contextWindow": context, "activeContextWindow": active, "contextChoices": localContextChoices(m.ID), "contextPending": context != active,
			"recommendedContextWindow": recommendedLocalContext(total, m.ID)})
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
	total, _, err := l.hardware()
	if err != nil {
		return err
	}
	if reason := localModelInstallReason(total, model); reason != "" {
		return errors.New(reason)
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
	l.runtimeContext = 0
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
	previousContext := l.contextSize(previous.LocalModel)
	changed := false
	runtimeAttempted := false
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
		}
		if runtimeAttempted && previous.LocalModel != "" {
			if err := l.startRuntimeAtContext(l.app.ctx, previous.LocalModel, previousContext); err != nil {
				result = errors.Join(result, fmt.Errorf("Restore previous local model: %w", err))
			} else {
				result = errors.Join(result, l.writeCatalog(previous.LocalModel))
			}
		}
		if changed {
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
	runtimeAttempted = true
	if err := l.startRuntime(ctx, id); err != nil {
		return err
	}
	if err := l.checkModel(ctx, id); err != nil {
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
	if !previous.active() && !appStoreBuild {
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

const localMaxOutputTokens = 4096

func (l *localManager) configuredContextSize(id string) int {
	l.mu.Lock()
	configured := l.contexts[id]
	l.mu.Unlock()
	if configured != 0 {
		return configured
	}
	total, _, _ := localHardware()
	return recommendedLocalContext(total, id)
}
func (l *localManager) contextSize(id string) int {
	l.mu.Lock()
	active := 0
	if l.runtimeModel == id && l.endpoint != "" {
		active = l.runtimeContext
	}
	l.mu.Unlock()
	if active > 0 {
		return active
	}
	return l.configuredContextSize(id)
}
func (l *localManager) models(id string) []object {
	m, ok := findLocalModel(id)
	if !ok {
		return nil
	}
	window := l.contextSize(id)
	return []object{{"id": id, "name": m.Name, "vendor": "Local", "model_picker_enabled": true, "supported_endpoints": []any{"/responses", "/chat/completions"}, "capabilities": object{"type": "chat", "limits": object{"max_context_window_tokens": window, "max_prompt_tokens": window - localMaxOutputTokens, "max_output_tokens": localMaxOutputTokens}, "supports": object{"streaming": true, "tool_calls": true}}}}
}
func (l *localManager) writeCatalog(id string) error {
	m := fallbackModel(id, localModelName(id), 0)
	window := l.contextSize(id)
	m["description"] = "Local · llama.cpp"
	m["context_window"] = window
	m["max_context_window"] = window
	m["effective_context_window_percent"] = (window - localMaxOutputTokens) * 100 / window
	m["input_modalities"] = []any{"text"}
	m["supports_image_detail_original"] = false
	m["supports_search_tool"] = false
	m["support_verbosity"] = false
	m["default_reasoning_summary"] = "none"
	m["use_responses_lite"] = false
	return writeJSON(l.app.path("codex-model-catalog.json"), object{"models": []any{m}})
}
func localSecret() string { return rand.Text() }
