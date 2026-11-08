package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalRecommendation(t *testing.T) {
	for _, c := range []struct {
		total, available int64
		want             string
	}{{16, 10, "qwen3-8b"}, {32, 16, "qwen3-8b"}, {36, 16, "qwen3-8b"}, {32, 28, "qwen3-coder-30b"}, {48, 40, "qwen3-coder-30b"}, {64, 56, "qwen3-coder-30b"}, {64, 10, "qwen3-8b"}, {32, 7, "qwen3-4b"}, {16, 4, ""}} {
		if got := recommendedLocalModel(c.total<<30, c.available<<30); got != c.want {
			t.Fatalf("RAM %d/%d: %s != %s", c.available, c.total, got, c.want)
		}
	}
}
func TestLocalModelDownloadEligibilityDoesNotRequireIdleMemory(t *testing.T) {
	coder, _ := findLocalModel("qwen3-coder-30b")
	if reason := localModelInstallReason(32<<30, coder); reason != "" {
		t.Fatal("32 GB Mac cannot select the coding model", reason)
	}
	if localModelFits(32<<30, 16<<30, coder) {
		t.Fatal("runtime recommendation ignored current memory pressure")
	}
	if reason := localModelInstallReason(16<<30, coder); reason == "" {
		t.Fatal("download selection ignored the minimum total memory")
	}
}
func TestLocalPackageEnvironmentUsesOnlyMicrosoftFeed(t *testing.T) {
	t.Setenv("UV_INDEX", "https://pypi.org/simple/")
	t.Setenv("UV_EXTRA_INDEX_URL", "https://files.pythonhosted.org/")
	t.Setenv("PIP_INDEX_URL", "https://pypi.org/simple/")
	t.Setenv("PIP_EXTRA_INDEX_URL", "https://files.pythonhosted.org/")
	t.Setenv("PIP_TRUSTED_HOST", "files.pythonhosted.org")
	t.Setenv("PIP_CONFIG_FILE", "/untrusted/pip.conf")
	a := localTestApp(t)
	env := map[string]string{}
	for _, entry := range a.local.environment() {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	if env["UV_DEFAULT_INDEX"] != localPackageIndex || env["PIP_INDEX_URL"] != localPackageIndex || env["PIP_CONFIG_FILE"] != os.DevNull {
		t.Fatal("runtime or nested builds did not use the isolated Microsoft index")
	}
	for _, key := range []string{"UV_INDEX", "UV_EXTRA_INDEX_URL", "PIP_EXTRA_INDEX_URL", "PIP_TRUSTED_HOST"} {
		if _, ok := env[key]; ok {
			t.Fatalf("installer retained unsafe package override %s", key)
		}
	}
}
func TestLocalMicrosoftPackageIndex(t *testing.T) {
	wheel := "https://ms-feed-25.pkgs.visualstudio.com/org/_packaging/feed/pypi/download/demo/1.0/demo-1.0-py3-none-any.whl#sha256=abc123"
	source := `<a href="` + wheel + `" data-requires-python=">=3.12" data-core-metadata="sha256=def456" data-dist-info-metadata='sha256=def456'>demo</a>`
	a := localTestApp(t)
	var calls atomic.Int32
	a.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.String() != localPackageIndex+"demo/" || r.Header.Get("Accept") != "text/html" {
			t.Fatal("installer requested a different feed or index format", r.URL, r.Header)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(source))}, nil
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	index, closeIndex, err := a.local.packageIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer closeIndex()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	for _, test := range []struct {
		name string
		code int
	}{{"demo/", 200}, {"nested/path/", 404}, {"demo/?extra-index=https://pypi.org", 404}} {
		response, err := client.Get(index + test.name)
		if err != nil {
			t.Fatal(err)
		}
		data, err := readResponse(response, 1<<20)
		if err != nil || response.StatusCode != test.code {
			t.Fatal("index request returned an unexpected result", err, response.StatusCode)
		}
		if test.code == 200 && (!strings.Contains(string(data), wheel) ||
			!strings.Contains(string(data), `data-requires-python=">=3.12"`) ||
			strings.Contains(string(data), "data-core-metadata") || strings.Contains(string(data), "data-dist-info-metadata")) {
			t.Fatal("index lost the original wheel URL, checksum, or Python requirement", string(data))
		}
	}
	if calls.Load() != 1 {
		t.Fatal("invalid local requests reached the package feed")
	}
}
func TestLocalMicrosoftPackageIndexRejectsExternalSources(t *testing.T) {
	for _, address := range []string{
		"https://files.pythonhosted.org/demo.whl",
		"http://packagefeedproxy.microsoft.io/demo.whl",
		"https://packagefeedproxy.microsoft.io:444/demo.whl",
		"https://ms-feed-25.pkgs.visualstudio.com.evil.invalid/demo.whl",
		"https://user@packagefeedproxy.microsoft.io/demo.whl",
	} {
		if _, err := localPackageHTML([]byte(`<a href="` + address + `">demo</a>`)); err == nil {
			t.Fatal("index accepted an unapproved source", address)
		}
	}
	for _, address := range []string{
		"https://packagefeedproxy.microsoft.io/demo.whl#sha256=abc",
		"https://ms-feed-25.pkgs.visualstudio.com/demo.whl#sha256=abc",
		"https://example-blob.vsblob.vsassets.io/demo.whl#sha256=abc",
	} {
		if _, err := localPackageHTML([]byte(`<a href="` + address + `">demo</a>`)); err != nil {
			t.Fatal("index rejected an approved Microsoft source", err)
		}
	}
}
func TestLocalAccountIsolation(t *testing.T) {
	s := session{Method: "local", LocalModel: "qwen3-4b", Username: "Qwen3 4B", LocalModels: []string{"qwen3-4b"}, Accounts: []account{{Username: "github-user", Token: "secret"}}}
	if !s.active() || len(savedAccounts(s)) != 1 {
		t.Fatal("Local must be active without becoming a GitHub credential")
	}
	values := publicAccounts(s)
	if len(values) != 2 || values[1]["selected"] != true || strings.Contains(string(jsonBytes(values)), "secret") {
		t.Fatal("Invalid public account list", values)
	}
}
func TestLocalCodexCatalogUsesSupportedPatchToolType(t *testing.T) {
	a := localTestApp(t)
	for _, id := range []string{"qwen3-4b", "qwen3-8b", "qwen3-coder-30b"} {
		t.Run(id, func(t *testing.T) {
			if err := a.local.writeCatalog(id); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(a.path("codex-model-catalog.json"))
			if err != nil {
				t.Fatal(err)
			}
			var catalog object
			if err := json.Unmarshal(data, &catalog); err != nil {
				t.Fatal(err)
			}
			models := arr(catalog["models"])
			if len(models) != 1 {
				t.Fatalf("expected one local model, got %d", len(models))
			}
			model := obj(models[0])
			if model["slug"] != id || model["apply_patch_tool_type"] != "freeform" {
				t.Fatal("local catalog must use Codex's supported freeform patch tool", model)
			}
			if model["support_verbosity"] != false || model["supports_search_tool"] != false ||
				model["supports_image_detail_original"] != false || model["use_responses_lite"] != false ||
				model["default_reasoning_summary"] != "none" {
				t.Fatal("local catalog lost its capability overrides", model)
			}
			modalities := arr(model["input_modalities"])
			if len(modalities) != 1 || modalities[0] != "text" ||
				number(model["context_window"]) != float64(a.local.contextSize(id)) ||
				model["max_context_window"] != model["context_window"] {
				t.Fatal("local catalog lost its input or context limits", model)
			}
		})
	}
}

func TestLocalConfigRestoresUserModel(t *testing.T) {
	original := "model = \"original\"\nnotify = [\"custom\"]\n\n[mcp_servers.user]\ncommand=\"test\"\n"
	injected := injectLocalModelConfig(original, "qwen3-4b")
	if !strings.Contains(injected, "# Kobashi local") || strings.Contains(injected, "original") {
		t.Fatal(injected)
	}
	if restored := restoreLocalModelConfig(injected, original); restored != original {
		t.Fatalf("restored %q", restored)
	}
	userChange := strings.Replace(injected, "\"qwen3-4b\" # Kobashi local", "\"user-new\"", 1)
	if restored := restoreLocalModelConfig(userChange, original); restored != userChange {
		t.Fatal("overwrote user choice")
	}
}
func TestLocalContextBudget(t *testing.T) {
	a := localTestApp(t)
	total, _, _ := localHardware()
	for _, id := range []string{"qwen3-4b", "qwen3-8b", "qwen3-coder-30b"} {
		t.Run(id, func(t *testing.T) {
			window := a.local.contextSize(id)
			if id == "qwen3-8b" && total >= 16<<30 && (window != 40960 || 34856+localMaxOutputTokens > window) {
				t.Fatal("Qwen3 8B must fit the reported first prompt and a full reply within its native context")
			}
			if window != recommendedLocalContext(total, id) {
				t.Fatal("default does not use the RAM-based recommendation", id, window)
			}
			limits := obj(nested(a.local.models(id)[0], "capabilities", "limits"))
			if limits["max_context_window_tokens"] != window ||
				limits["max_prompt_tokens"] != window-localMaxOutputTokens ||
				limits["max_output_tokens"] != localMaxOutputTokens {
				t.Fatal("client context budget must reserve output space", limits)
			}
			if err := a.local.runtimeConfig(id, 12345, "private", window); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(a.local.path("server", id, "model_settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			var settings object
			if err := json.Unmarshal(data, &settings); err != nil {
				t.Fatal(err)
			}
			if number(nested(settings, "models", id, "max_context_window")) != float64(window) ||
				number(nested(settings, "models", id, "max_tokens")) != localMaxOutputTokens {
				t.Fatal("runtime and client context budgets diverged", settings)
			}
			if err := a.local.writeCatalog(id); err != nil {
				t.Fatal(err)
			}
			data, err = os.ReadFile(a.path("codex-model-catalog.json"))
			if err != nil {
				t.Fatal(err)
			}
			var catalog object
			if err := json.Unmarshal(data, &catalog); err != nil {
				t.Fatal(err)
			}
			model := obj(arr(catalog["models"])[0])
			percent := number(model["effective_context_window_percent"])
			if number(model["context_window"]) != float64(window) ||
				percent != float64((window-localMaxOutputTokens)*100/window) ||
				float64(window)*percent/100+localMaxOutputTokens > float64(window) {
				t.Fatal("Codex must reserve reply space inside the runtime limit", model)
			}
		})
	}
}
func TestLocalContextRecommendations(t *testing.T) {
	for _, row := range []struct {
		ram                  int64
		small, medium, coder int
	}{
		{8, 8192, 8192, 16384},
		{16, 32768, 40960, 16384},
		{24, 65536, 40960, 16384},
		{32, 131072, 40960, 16384},
		{36, 131072, 40960, 16384},
		{48, 131072, 40960, 65536},
		{64, 262144, 40960, 131072},
		{96, 262144, 40960, 262144},
		{128, 262144, 40960, 262144},
	} {
		for id, want := range map[string]int{"qwen3-4b": row.small, "qwen3-8b": row.medium, "qwen3-coder-30b": row.coder} {
			if got := recommendedLocalContext(row.ram<<30, id); got != want {
				t.Fatalf("%d GiB %s: got %d, want %d", row.ram, id, got, want)
			}
		}
	}
	for _, model := range localModels {
		previous := 0
		for ram := int64(0); ram <= 256; ram++ {
			got := recommendedLocalContext(ram<<30, model.ID)
			if !slices.Contains(localContextChoices(model.ID), got) || got < previous || got > model.MaxContext {
				t.Fatal("recommendation is not a monotonic supported native capacity", ram, model.ID, got)
			}
			previous = got
		}
		for _, tier := range []int64{16, 24, 32, 48, 64, 96} {
			if recommendedLocalContext((tier<<30)-1, model.ID) != recommendedLocalContext((tier-1)<<30, model.ID) {
				t.Fatal("RAM threshold rounded up before reaching the next tier", tier, model.ID)
			}
		}
	}
	if recommendedLocalContext(32<<30, "unknown") != 0 {
		t.Fatal("unknown model received a recommendation")
	}
}
func TestLocalContextRecommendedDefaultsPreserveSelections(t *testing.T) {
	a := localTestApp(t)
	catalog := a.local.catalog()
	total := int64(number(catalog["ram"]))
	for _, entry := range arr(catalog["models"]) {
		model := obj(entry)
		id := str(model["id"])
		want := recommendedLocalContext(total, id)
		if model["recommendedContextWindow"] != want || model["contextWindow"] != want {
			t.Fatal("catalog did not publish the RAM-based default and recommendation", model)
		}
	}
	id := "qwen3-4b"
	if _, err := a.local.setContext(id, 8192); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(a.options)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.cancel()
	if reopened.local.configuredContextSize(id) != 8192 {
		t.Fatal("recommendation overwrote the saved manual choice on relaunch")
	}
	a.local.runtimeModel, a.local.runtimeContext, a.local.endpoint = id, 32768, "http://127.0.0.1:1"
	for _, entry := range arr(a.local.catalog()["models"]) {
		model := obj(entry)
		if model["id"] == id && (model["contextWindow"] != 8192 || model["activeContextWindow"] != 32768 ||
			model["recommendedContextWindow"] != recommendedLocalContext(total, id)) {
			t.Fatal("recommendation overrode configured or running capacity", model)
		}
	}
}
func localTestApp(t *testing.T) *App {
	t.Helper()
	home := t.TempDir()
	a, err := New(Options{Home: home, CodexHome: filepath.Join(home, ".codex"), ReadEnvironment: func(string) (*string, error) { return nil, nil }, WriteEnvironment: func(string, *string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.cancel)
	return a
}
func TestLocalContextSettingsPersistenceAndBudgets(t *testing.T) {
	a := localTestApp(t)
	for _, model := range localModels {
		choices := localContextChoices(model.ID)
		if choices[0] != 8192 || choices[len(choices)-1] != model.MaxContext {
			t.Fatal("slider does not cover the supported native range", model.ID, choices)
		}
		for _, window := range choices {
			if _, err := a.local.setContext(model.ID, window); err != nil {
				t.Fatal(err)
			}
			limits := obj(nested(a.local.models(model.ID)[0], "capabilities", "limits"))
			if limits["max_context_window_tokens"] != window ||
				limits["max_prompt_tokens"] != window-localMaxOutputTokens {
				t.Fatal("configured client budget is inconsistent", model.ID, limits)
			}
			if err := a.local.runtimeConfig(model.ID, 12345, "synthetic", window); err != nil {
				t.Fatal(err)
			}
			var settings object
			data, err := os.ReadFile(a.local.path("server", model.ID, "model_settings.json"))
			if err != nil || json.Unmarshal(data, &settings) != nil ||
				number(nested(settings, "models", model.ID, "max_context_window")) != float64(window) {
				t.Fatal("runtime did not receive the chosen context", err, settings)
			}
			if err := a.local.writeCatalog(model.ID); err != nil {
				t.Fatal(err)
			}
			var catalog object
			data, err = os.ReadFile(a.path("codex-model-catalog.json"))
			if err != nil || json.Unmarshal(data, &catalog) != nil ||
				number(nested(obj(arr(catalog["models"])[0]), "context_window")) != float64(window) {
				t.Fatal("Codex did not receive the chosen context", err, catalog)
			}
		}
	}
	if err := a.saveSession(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(a.options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.cancel)
	for _, model := range localModels {
		if reopened.local.contextSize(model.ID) != model.MaxContext {
			t.Fatal("per-model settings did not survive a session save and relaunch", model.ID)
		}
	}
	info, err := os.Stat(a.path("local-context.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("context settings lost private file permissions", err)
	}
}
func TestLocalContextApplyDoesNotInterruptActiveRuntime(t *testing.T) {
	a := localTestApp(t)
	id, active, target := "qwen3-4b", 32768, 262144
	a.state.LocalModel, a.state.Method = id, "local"
	if err := a.local.runtimeConfig(id, 12345, "synthetic", active); err != nil {
		t.Fatal(err)
	}
	path := a.local.path("server", id, "model_settings.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cancelled atomic.Bool
	var effective atomic.Int64
	effective.Store(int64(active))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Error("runtime context update is not authenticated")
			http.Error(w, "unauthorized", 401)
			return
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/admin/api/models/"+id+"/settings":
			body, err := readObject(r.Body, 4096)
			if err != nil || len(body) != 1 {
				t.Error("context update must change only max_context_window", body, err)
				http.Error(w, "invalid", 400)
				return
			}
			value := int(number(body["max_context_window"]))
			var settings object
			if err := json.Unmarshal(before, &settings); err != nil {
				t.Error(err)
				http.Error(w, "invalid settings", 500)
				return
			}
			obj(nested(settings, "models", id))["max_context_window"] = value
			if err := writeJSON(path, settings); err != nil {
				t.Error(err)
				http.Error(w, "write failed", 500)
				return
			}
			effective.Store(int64(value))
			respond(w, 200, object{"success": true, "model_id": id, "requires_reload": false,
				"settings": object{"max_context_window": value}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models/status":
			respond(w, 200, object{"models": []any{object{"id": id, "max_context_window": effective.Load()}}})
		default:
			t.Error("context application must not call inference or restart the runtime", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 404)
		}
	}))
	defer server.Close()
	a.local.runtimeModel, a.local.runtimeContext = id, active
	a.local.endpoint, a.local.secret = server.URL, "synthetic"
	a.local.runtimeCancel = func() { cancelled.Store(true) }
	a.routes.RLock()
	response := controlCall(a, "/api/local/context", object{"model": id, "contextWindow": target})
	a.routes.RUnlock()
	var result object
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	var settings object
	if response.Code != 200 || result["contextPending"] != false ||
		number(result["activeContextWindow"]) != float64(target) ||
		cancelled.Load() || a.local.endpoint != server.URL ||
		err != nil || bytes.Equal(before, after) || json.Unmarshal(after, &settings) != nil ||
		number(nested(settings, "models", id, "max_context_window")) != float64(target) ||
		nested(settings, "models", id, "turboquant_kv_enabled") != true ||
		number(nested(settings, "models", id, "turboquant_kv_bits")) != 4 {
		t.Fatal("context was not applied in place without changing KV compression", response.Code, result, err)
	}
	if a.local.contextSize(id) != target || a.local.configuredContextSize(id) != target ||
		nested(a.local.models(id)[0], "capabilities", "limits", "max_context_window_tokens") != target ||
		nested(a.local.models(id)[0], "capabilities", "limits", "max_prompt_tokens") != target-localMaxOutputTokens {
		t.Fatal("runtime and client budgets were not updated together")
	}
	var catalog object
	data, err := os.ReadFile(a.path("codex-model-catalog.json"))
	if err != nil || json.Unmarshal(data, &catalog) != nil ||
		number(obj(arr(catalog["models"])[0])["context_window"]) != float64(target) {
		t.Fatal("Codex catalog did not immediately receive the effective runtime capacity", err, catalog)
	}
	entries := arr(a.local.catalog()["models"])
	for _, entry := range entries {
		model := obj(entry)
		if model["id"] == id && (model["contextWindow"] != target ||
			model["activeContextWindow"] != target || model["contextPending"] != false) {
			t.Fatal("context metadata does not confirm immediate application", model)
		}
	}
	a.local.mu.Lock()
	a.local.runtimeModel, a.local.endpoint, a.local.runtimeContext = "", "", 0
	a.local.mu.Unlock()
	if a.local.contextSize(id) != target {
		t.Fatal("the next runtime did not use its saved context")
	}
	if a.local.contextSize("qwen3-8b") != 40960 {
		t.Fatal("changing one model changed another model's default")
	}
}
func TestLocalContextApplyFailuresRestorePreviousCapacity(t *testing.T) {
	for _, failure := range []string{"rejected", "invalid JSON", "wrong setting", "missing reload confirmation", "reload required", "wrong effective limit"} {
		t.Run(failure, func(t *testing.T) {
			a := localTestApp(t)
			id, active, target := "qwen3-4b", 32768, 65536
			a.state.LocalModel, a.state.Method = id, "local"
			if _, err := a.local.setContext(id, active); err != nil {
				t.Fatal(err)
			}
			if err := a.local.writeCatalog(id); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(a.path("codex-model-catalog.json"))
			if err != nil {
				t.Fatal(err)
			}
			var effective atomic.Int64
			effective.Store(int64(active))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					value := effective.Load()
					if failure == "wrong effective limit" && value == int64(target) {
						value = int64(active)
					}
					respond(w, 200, object{"models": []any{object{"id": id, "max_context_window": value}}})
					return
				}
				body, err := readObject(r.Body, 4096)
				if err != nil {
					t.Error(err)
					http.Error(w, "invalid", 400)
					return
				}
				value := int(number(body["max_context_window"]))
				effective.Store(int64(value))
				result := object{"success": true, "model_id": id, "requires_reload": false,
					"settings": object{"max_context_window": value}}
				if value == target {
					switch failure {
					case "rejected":
						http.Error(w, "rejected", 400)
						return
					case "invalid JSON":
						w.Write([]byte("{"))
						return
					case "wrong setting":
						result["settings"] = object{"max_context_window": active}
					case "missing reload confirmation":
						delete(result, "requires_reload")
					case "reload required":
						result["requires_reload"] = true
					}
				}
				respond(w, 200, result)
			}))
			defer server.Close()
			var cancelled atomic.Bool
			a.local.runtimeModel, a.local.runtimeContext = id, active
			a.local.endpoint, a.local.secret = server.URL, "synthetic"
			a.local.runtimeCancel = func() { cancelled.Store(true) }
			response := controlCall(a, "/api/local/context", object{"model": id, "contextWindow": target})
			after, err := os.ReadFile(a.path("codex-model-catalog.json"))
			if response.Code != 400 || !strings.Contains(response.Body.String(), "Could not apply local context") ||
				cancelled.Load() || effective.Load() != int64(active) ||
				a.local.contextSize(id) != active || a.local.configuredContextSize(id) != active ||
				err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed application did not preserve the old runtime and client budgets", response.Body.String(), err)
			}
			reopened, err := New(a.options)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.cancel()
			if reopened.local.configuredContextSize(id) != active {
				t.Fatal("failed context application was persisted")
			}
		})
	}
}
func TestLocalContextSettingsRejectInvalidInputAndFailedWrites(t *testing.T) {
	a := localTestApp(t)
	request := httptest.NewRequest("POST", "/api/local/context", strings.NewReader(`{"model":"qwen3-4b","contextWindow":65536}`))
	response := httptest.NewRecorder()
	a.control(response, request)
	if response.Code != 401 {
		t.Fatal("context settings accepted an unauthenticated write")
	}
	for _, body := range []object{
		{"model": "unknown", "contextWindow": 32768},
		{"model": "qwen3-8b", "contextWindow": 65536},
		{"model": "qwen3-4b", "contextWindow": 0},
		{"model": "qwen3-4b", "contextWindow": 8191},
		{"model": "qwen3-4b", "contextWindow": 65536.5},
		{"model": "qwen3-4b", "contextWindow": "65536"},
	} {
		response := controlCall(a, "/api/local/context", body)
		if response.Code != 400 || !strings.Contains(response.Body.String(), "error") {
			t.Fatal("invalid context did not return an actionable error", body, response.Code, response.Body.String())
		}
	}
	if _, err := os.Stat(a.path("local-context.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rejected context was persisted", err)
	}
	if err := os.MkdirAll(a.path("local-context.json"), 0700); err != nil {
		t.Fatal(err)
	}
	original := a.local.configuredContextSize("qwen3-4b")
	if _, err := a.local.setContext("qwen3-4b", 65536); err == nil ||
		a.local.configuredContextSize("qwen3-4b") != original {
		t.Fatal("failed persistence changed in-memory context")
	}
}
func TestLocalContextSettingsLoadErrorsAreVisible(t *testing.T) {
	for _, data := range []string{`{`, `{"unknown":32768}`, `{"qwen3-8b":65536}`, `{"qwen3-4b":0}`} {
		t.Run(data, func(t *testing.T) {
			a := localTestApp(t)
			if err := os.MkdirAll(a.path(""), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(a.path("local-context.json"), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			reopened, err := New(a.options)
			if err == nil || reopened != nil {
				t.Fatal("invalid saved settings silently fell back to a default", data, err)
			}
		})
	}
}
func TestLocalProxyIsolationAndStreaming(t *testing.T) {
	a := localTestApp(t)
	a.state = session{LocalModel: "qwen3-4b", Username: "Qwen3 4B", Method: "local", Codex: true, Claude: true, Copilot: true}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-secret" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Github-Token") != "" {
			t.Error("credentials leaked", r.Header)
		}
		var body object
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "qwen3-4b" {
			t.Error("wrong model", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\"}\n\n")
	}))
	defer upstream.Close()
	a.local.endpoint = upstream.URL
	a.local.secret = "local-secret"
	for _, c := range []struct {
		handler     http.HandlerFunc
		path, model string
	}{{a.codex, "/v1/responses", "qwen3-4b"}, {a.copilotApp, "/v1/chat/completions", "qwen3-4b"}, {a.claude, "/v1/messages", "claude-sonnet-4-6"}} {
		req := httptest.NewRequest("POST", c.path, strings.NewReader(string(jsonBytes(object{"model": c.model, "stream": true}))))
		req.Header.Set("Authorization", "Bearer PROXY_MANAGED")
		req.Header.Set("Cookie", "private")
		req.Header.Set("X-Github-Token", "private")
		result := httptest.NewRecorder()
		c.handler(result, req)
		if result.Code != 200 || !strings.Contains(result.Body.String(), "response.completed") {
			t.Fatalf("%s: %d %s", c.path, result.Code, result.Body.String())
		}
	}
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"cloud-model"}`))
	req.Header.Set("Authorization", "Bearer PROXY_MANAGED")
	result := httptest.NewRecorder()
	a.codex(result, req)
	if result.Code != 400 {
		t.Fatal("silently replaced model")
	}
}
func TestLocalDisconnectWaitsForRequest(t *testing.T) {
	a := localTestApp(t)
	a.state = session{LocalModel: "qwen3-4b", LocalModels: []string{"qwen3-4b"}}
	a.routes.RLock()
	err := a.disconnect()
	a.routes.RUnlock()
	if err == nil || a.snapshot().LocalModel == "" {
		t.Fatal("disconnected active request")
	}
	if err := a.disconnect(); err != nil {
		t.Fatal(err)
	}
	if a.snapshot().active() || len(a.snapshot().LocalModels) != 1 {
		t.Fatal("did not preserve saved local model")
	}
}
func TestLocalRuntimeConfiguration(t *testing.T) {
	a := localTestApp(t)
	if err := a.local.runtimeConfig("qwen3-4b", 12345, "private", a.local.contextSize("qwen3-4b")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(a.local.path("server", "qwen3-4b", "model_settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var body object
	json.Unmarshal(b, &body)
	if nested(body, "models", "qwen3-4b", "turboquant_kv_scheme") != "turboquant" || number(nested(body, "models", "qwen3-4b", "turboquant_kv_bits")) != 4 {
		t.Fatal(string(b))
	}
}

func TestLocalInstallProgressUsesCompletedStepsAndDownloadBytes(t *testing.T) {
	a := localTestApp(t)
	a.local.installProgress(3, "oMLX")
	a.local.progress("Installing", 40, 100)
	state := a.local.status()
	if state.CompletedSteps != 3 || state.TotalSteps != 6 || state.Detail != "oMLX" ||
		state.Done != 40 || state.Total != 100 {
		t.Fatal("download overwrote completed installation steps", state)
	}
	a.local.installProgress(4, "Runtime packages")
	state = a.local.status()
	if state.Done != 0 || state.Total != 0 || state.CompletedSteps != 4 {
		t.Fatal("new step retained stale download progress", state)
	}
	a.local.progress("Downloading", 12, 80)
	state = a.local.status()
	if state.TotalSteps != 0 || state.Detail != "" || state.Done != 12 || state.Total != 80 {
		t.Fatal("model download retained runtime installation steps", state)
	}
}

func TestLocalInstallerOutputCountsPackagesAndIdentifiesFailedHost(t *testing.T) {
	a := localTestApp(t)
	output := &localInstallOutput{manager: a.local}
	for _, chunk := range []string{" Download", "ed wheel\nDownloaded setuptools\n", "cause: Failed to fetch: https://files.pythonhosted.org/private?token=secret"} {
		if _, err := output.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	output.flush()
	if a.local.status().Packages != 2 || !output.networkFailure || output.host != "files.pythonhosted.org" {
		t.Fatal("lost split output or exposed a URL instead of its host", a.local.status(), output.host)
	}
}

func TestLocalDownloadReportsFinalBytesAndHonorsCache(t *testing.T) {
	a := localTestApp(t)
	data := []byte("downloaded runtime fixture")
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	checksum := fileSHA(source)
	target := filepath.Join(t.TempDir(), "runtime")
	calls := 0
	a.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls > 1 {
			return nil, errors.New("cached file caused another download")
		}
		return &http.Response{StatusCode: 200, ContentLength: -1, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}
	a.local.installProgress(0, "Installer")
	if err := a.local.download(context.Background(), "https://example.invalid/runtime", checksum, target, "Installing"); err != nil {
		t.Fatal(err)
	}
	state := a.local.status()
	if state.Done != int64(len(data)) || state.Total != int64(len(data)) ||
		state.CompletedSteps != 0 || state.TotalSteps != 6 || fileSHA(target) != checksum {
		t.Fatal("small or unknown-length download lost its final progress", state)
	}
	if err := a.local.download(context.Background(), "https://example.invalid/runtime", checksum, target, "Installing"); err != nil || calls != 1 {
		t.Fatal("verified download was not reused", err)
	}
}

func localExecutableFixture(t *testing.T, path, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("local runtime executable fixtures require a Unix shell")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
}

func TestLocalRuntimeReusesVerifiedInstallationWithoutInstaller(t *testing.T) {
	for _, marker := range []string{"", localWheelHash, "outdated-marker"} {
		t.Run(marker, func(t *testing.T) {
			a := localTestApp(t)
			probes := filepath.Join(t.TempDir(), "probes")
			localExecutableFixture(t, a.local.python(), fmt.Sprintf("printf '%%s\\n' \"$2\" >> %q\nexit 0", probes))
			path := a.local.path("runtime-"+localRuntimeVersion, ".kobashi-ready")
			if marker != "" {
				if err := writeAtomic(path, []byte(marker)); err != nil {
					t.Fatal(err)
				}
			}
			a.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				t.Error("verified runtime attempted a download", r.URL)
				return nil, errors.New("unexpected download")
			})}
			if err := a.local.installRuntime(context.Background()); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != localWheelHash {
				t.Fatal("verified runtime completion was not saved", err)
			}
			data, err = os.ReadFile(probes)
			if err != nil || !strings.Contains(string(data), "version('omlx') == \"0.7.3\"") ||
				!strings.Contains(string(data), "import omlx.server") || a.local.status().CompletedSteps != 6 ||
				a.local.status().Detail != "Model tools ready" {
				t.Fatal("runtime was reused without checking version and imports", err, string(data))
			}
			if _, err := os.Stat(a.local.path("uv")); !os.IsNotExist(err) {
				t.Fatal("complete runtime required an installer", err)
			}
		})
	}
}

func TestLocalPythonReusesExistingEnvironment(t *testing.T) {
	a := localTestApp(t)
	localExecutableFixture(t, a.local.python(), "exit 0")
	if err := a.local.preparePython(context.Background(), "/missing/uv"); err != nil {
		t.Fatal("existing Python caused an installer invocation", err)
	}
	if a.local.status().Detail != "Reusing Python setup" {
		t.Fatal("existing Python was described as a new installation", a.local.status())
	}
}

func TestLocalPythonDiscoveryBeforeInstallation(t *testing.T) {
	for _, scenario := range []string{"existing", "missing", "discovery-error", "incompatible"} {
		t.Run(scenario, func(t *testing.T) {
			a := localTestApp(t)
			base := filepath.Join(t.TempDir(), "python3")
			script := "exit 0"
			if scenario == "incompatible" {
				script = "exit 1"
			}
			localExecutableFixture(t, base, script)
			calls := filepath.Join(t.TempDir(), "calls")
			installed := filepath.Join(t.TempDir(), "installed")
			find := fmt.Sprintf("printf '%%s\\n' %q", base)
			if scenario == "missing" {
				find = fmt.Sprintf("if [ -f %q ]; then printf '%%s\\n' %q; else echo 'error: No interpreter found for Python 3.12' >&2; exit 2; fi", installed, base)
			} else if scenario == "discovery-error" {
				find = "echo 'error: permission denied' >&2; exit 2"
			}
			localExecutableFixture(t, a.local.path("uv"), fmt.Sprintf(`
printf '%%s\n' "$*" >> %q
case "$1 $2" in
  "python find") %s ;;
  "python install") touch %q ;;
  "venv --no-python-downloads") mkdir -p "$5/bin"; cp "$4" "$5/bin/python3" ;;
  *) exit 3 ;;
esac`, calls, find, installed))
			err := a.local.preparePython(context.Background(), a.local.path("uv"))
			wantError := scenario == "discovery-error" || scenario == "incompatible"
			if (err != nil) != wantError {
				t.Fatalf("unexpected setup result: %v", err)
			}
			data, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if !strings.HasPrefix(lines[0], "python find --offline --no-python-downloads --no-project --system 3.12") {
				t.Fatal("installation preceded offline discovery", lines)
			}
			if strings.Contains(string(data), "python install") != (scenario == "missing") {
				t.Fatal("Python was installed despite an existing interpreter or a discovery error", lines)
			}
			if strings.Contains(string(data), "venv ") != !wantError {
				t.Fatal("environment was recreated without a verified interpreter", lines)
			}
		})
	}
}

func TestLocalRuntimeRejectsStaleCompletionMarker(t *testing.T) {
	a := localTestApp(t)
	localExecutableFixture(t, a.local.python(), `case "$2" in *"import omlx.server"*) exit 1 ;; esac; exit 0`)
	localExecutableFixture(t, a.local.path("uv"), `[ "$1" = "--version" ] || exit 3`)
	marker := a.local.path("runtime-"+localRuntimeVersion, ".kobashi-ready")
	if err := writeAtomic(marker, []byte(localWheelHash)); err != nil {
		t.Fatal(err)
	}
	a.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("fixture stops before package downloads")
	})}
	err := a.local.installRuntime(context.Background())
	if err == nil || !strings.Contains(err.Error(), "fixture stops") {
		t.Fatal("stale completion marker skipped runtime validation", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("broken installation remained marked ready", err)
	}
}

func TestLocalSetupCommandFixture(t *testing.T) {
	if os.Getenv("KOBASHI_SETUP_COMMAND_FIXTURE") != "1" {
		return
	}
	if os.Getenv("HTTPS_PROXY") != "http://127.0.0.1:12345" ||
		os.Getenv("https_proxy") != "http://127.0.0.1:54321" {
		fmt.Println("proxy not inherited")
		os.Exit(2)
	}
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		value := os.Getenv(key)
		if !strings.Contains(value, "corp.invalid") || !strings.Contains(value, "service.invalid") ||
			!strings.Contains(value, "127.0.0.1") {
			fmt.Println("local package index did not bypass the external proxy")
			os.Exit(2)
		}
	}
	fmt.Println("Downloaded wheel")
	fmt.Println("cause: Failed to fetch: https://files.pythonhosted.org/wheel.metadata")
	os.Exit(1)
}

func TestLocalSetupCommandNamesFailingStepAndPreservesLogs(t *testing.T) {
	t.Setenv("KOBASHI_SETUP_COMMAND_FIXTURE", "1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:12345")
	t.Setenv("https_proxy", "http://127.0.0.1:54321")
	t.Setenv("NO_PROXY", "corp.invalid")
	t.Setenv("no_proxy", "service.invalid")
	a := localTestApp(t)
	if err := os.MkdirAll(a.local.path(), 0700); err != nil {
		t.Fatal(err)
	}
	a.local.installProgress(4, "Runtime packages")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = a.local.run(ctx, executable, "-test.run=^TestLocalSetupCommandFixture$", "--", "--default-index", "http://127.0.0.1:54321/simple/")
	if err == nil || !strings.Contains(err.Error(), "Runtime packages failed") ||
		!strings.Contains(err.Error(), "files.pythonhosted.org") || !strings.Contains(err.Error(), "network/proxy") {
		t.Fatal("setup lost its step or failed-host guidance", err)
	}
	data, err := os.ReadFile(a.local.path("setup.log"))
	if err != nil || !strings.Contains(string(data), "Failed to fetch:") || a.local.status().Packages != 1 {
		t.Fatal("setup output was not preserved or counted", err, string(data))
	}
}

// Opt-in real installation and inference, isolated from all installed clients.
func TestLocalInstalledRuntime(t *testing.T) {
	home := os.Getenv("KOBASHI_LOCAL_TEST_HOME")
	if home == "" {
		t.Skip("set KOBASHI_LOCAL_TEST_HOME to an isolated directory")
	}
	a, err := New(Options{Home: home, CodexHome: filepath.Join(home, ".codex"), ReadEnvironment: func(string) (*string, error) { return nil, nil }, WriteEnvironment: func(string, *string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.cancel(); a.local.close() })
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Minute)
	defer cancel()
	m, _ := findLocalModel("qwen3-4b")
	if err = a.local.prepare(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err = a.local.startRuntime(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if err = a.local.checkModel(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	original := `model = "original-model"
notify = ["custom"]
`
	if err = writeAtomic(filepath.Join(home, ".codex", "config.toml"), []byte(original)); err != nil {
		t.Fatal(err)
	}
	if err = a.local.activate(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	configured, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil || !strings.Contains(string(configured), "# Kobashi local") {
		t.Fatal("Codex was not configured", err)
	}
	claudeConfig, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil || !strings.Contains(string(claudeConfig), "PROXY_MANAGED") {
		t.Fatal("Claude was not configured", err)
	}
	a.mu.Lock()
	a.state.Copilot = true
	a.mu.Unlock()
	parameters := object{"type": "object", "properties": object{"value": object{"type": "string"}}, "required": []any{"value"}}
	tests := []struct {
		name, path string
		handler    http.HandlerFunc
		body       object
		end        string
	}{
		{"Codex", "/v1/responses", a.codex, object{"model": m.ID, "input": "Call kobashi_ready with value ready.", "tools": []any{object{"type": "function", "name": "kobashi_ready", "description": "Readiness check", "parameters": parameters}}, "tool_choice": "required", "max_output_tokens": 128, "stream": true}, "response.completed"},
		{"Claude", "/v1/messages", a.claude, object{"model": "claude-sonnet-4-6", "messages": []any{object{"role": "user", "content": "Call kobashi_ready with value ready."}}, "tools": []any{object{"name": "kobashi_ready", "description": "Readiness check", "input_schema": parameters}}, "tool_choice": object{"type": "tool", "name": "kobashi_ready"}, "max_tokens": 128, "stream": true}, "message_stop"},
		{"Copilot", "/v1/chat/completions", a.copilotApp, object{"model": m.ID, "messages": []any{object{"role": "user", "content": "Call kobashi_ready with value ready."}}, "tools": []any{object{"type": "function", "function": object{"name": "kobashi_ready", "description": "Readiness check", "parameters": parameters}}}, "tool_choice": "required", "max_tokens": 128, "stream": true}, "[DONE]"},
	}
	for _, test := range tests {
		test.body["temperature"] = 0
		switch test.name {
		case "Codex":
			test.body["instructions"] = localProbeInstructions
		case "Claude":
			test.body["system"] = localProbeInstructions
		case "Copilot":
			test.body["messages"] = append([]any{object{"role": "system", "content": localProbeInstructions}}, arr(test.body["messages"])...)
		}
		req := httptest.NewRequest("POST", test.path, strings.NewReader(string(jsonBytes(test.body)))).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer PROXY_MANAGED")
		result := httptest.NewRecorder()
		test.handler(result, req)
		if result.Code != 200 || !strings.Contains(result.Body.String(), "kobashi_ready") || !strings.Contains(result.Body.String(), test.end) {
			t.Errorf("%s streaming tools failed: %d %s", test.name, result.Code, result.Body.String())
			continue
		}
		callID := ""
		arguments := ""
		var responseCall object
		_ = readSSE(strings.NewReader(result.Body.String()), func(event sseEvent) error {
			value, _ := event.data()
			if value["type"] == "response.output_item.done" && nested(value, "item", "type") == "function_call" {
				responseCall = obj(value["item"])
				callID = str(responseCall["call_id"])
			}
			if value["type"] == "content_block_start" && nested(value, "content_block", "type") == "tool_use" {
				callID = str(nested(value, "content_block", "id"))
			}
			if value["type"] == "content_block_delta" {
				arguments += str(nested(value, "delta", "partial_json"))
			}
			for _, choice := range arr(value["choices"]) {
				for _, tool := range arr(nested(choice, "delta", "tool_calls")) {
					if id := str(nested(tool, "id")); id != "" {
						callID = id
					}
					arguments += str(nested(tool, "function", "arguments"))
				}
			}
			return nil
		})
		if callID == "" {
			t.Fatalf("%s did not provide a tool call ID", test.name)
		}
		follow := clone(test.body)
		follow["tool_choice"] = "none"
		switch test.name {
		case "Codex":
			follow["input"] = []any{object{"role": "user", "content": test.body["input"]}, responseCall, object{"type": "function_call_output", "call_id": callID, "output": "ready"}}
		case "Claude":
			var input object
			if json.Unmarshal([]byte(arguments), &input) != nil {
				t.Fatalf("Invalid Claude tool arguments: %s", arguments)
			}
			follow["tool_choice"] = object{"type": "none"}
			follow["messages"] = append(arr(test.body["messages"]), object{"role": "assistant", "content": []any{object{"type": "tool_use", "id": callID, "name": "kobashi_ready", "input": input}}}, object{"role": "user", "content": []any{object{"type": "tool_result", "tool_use_id": callID, "content": "ready"}}})
		case "Copilot":
			follow["messages"] = append(arr(test.body["messages"]), object{"role": "assistant", "tool_calls": []any{object{"id": callID, "type": "function", "function": object{"name": "kobashi_ready", "arguments": arguments}}}}, object{"role": "tool", "tool_call_id": callID, "content": "ready"})
		}
		req = httptest.NewRequest("POST", test.path, strings.NewReader(string(jsonBytes(follow)))).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer PROXY_MANAGED")
		result = httptest.NewRecorder()
		test.handler(result, req)
		if result.Code != 200 || !strings.Contains(result.Body.String(), test.end) || strings.Contains(result.Body.String(), "event: error") || strings.Contains(result.Body.String(), "response.failed") {
			t.Errorf("%s tool result failed: %d %s", test.name, result.Code, result.Body.String())
		}
		t.Log(test.name + " streaming tool call and result completed")
	}
	a.mu.Lock()
	a.state.Copilot = false
	a.mu.Unlock()
	if err = a.disconnect(); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil || strings.TrimSpace(string(restored)) != strings.TrimSpace(original) {
		t.Fatalf("Codex restoration failed: %v %q", err, restored)
	}
	if a.snapshot().active() || !contains(a.snapshot().LocalModels, m.ID) {
		t.Fatal("Disconnect lost the saved model")
	}
	t.Log("Automatic setup, client configuration, streaming tools, tool results, and restoration passed")
}

func TestLocalDownloadResumeAndIntegrity(t *testing.T) {
	a := localTestApp(t)
	data := []byte("verified runtime content")
	original := filepath.Join(t.TempDir(), "original")
	if err := os.WriteFile(original, data, 0600); err != nil {
		t.Fatal(err)
	}
	checksum := fileSHA(original)
	requested := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
		if r.Header.Get("Range") != "bytes=8-" {
			t.Error("download did not resume")
		}
		w.Header().Set("Content-Range", "bytes 8-23/24")
		w.WriteHeader(206)
		w.Write(data[8:])
	}))
	defer server.Close()
	a.client = server.Client()
	target := filepath.Join(t.TempDir(), "runtime")
	os.WriteFile(target+".part", data[:8], 0600)
	if err := a.local.download(context.Background(), server.URL, checksum, target, "Installing"); err != nil {
		t.Fatal(err)
	}
	if !requested || fileSHA(target) != checksum {
		t.Fatal("download integrity failed")
	}
	requested = false
	if err := a.local.download(context.Background(), server.URL, checksum, target, "Installing"); err != nil || requested {
		t.Fatal("redownloaded verified runtime", err)
	}
	corrupt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "corrupt") }))
	defer corrupt.Close()
	bad := filepath.Join(t.TempDir(), "bad")
	if err := a.local.download(context.Background(), corrupt.URL, checksum, bad, "Installing"); err == nil {
		t.Fatal("accepted corrupt runtime")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("published corrupt runtime")
	}
}
