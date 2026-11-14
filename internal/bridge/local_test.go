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
			args, err := a.local.runtimeArguments(id, 12345, window, "private")
			if err != nil || !strings.Contains(strings.Join(args, " "), "--ctx-size "+fmt.Sprint(window)) {
				t.Fatal("runtime context diverged", err, args)
			}
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
			args, err := a.local.runtimeArguments(model.ID, 12345, window, "synthetic")
			if err != nil || !strings.Contains(strings.Join(args, " "), "--ctx-size "+fmt.Sprint(window)) {
				t.Fatal("runtime did not receive chosen context", err, args)
			}
			if err := a.local.writeCatalog(model.ID); err != nil {
				t.Fatal(err)
			}
			var catalog object
			data, err := os.ReadFile(a.path("codex-model-catalog.json"))
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
func TestLocalInstallProgressUsesCompletedStepsAndDownloadBytes(t *testing.T) {
	a := localTestApp(t)
	a.local.installProgress(3, "llama.cpp")
	a.local.progress("Installing", 40, 100)
	state := a.local.status()
	if state.CompletedSteps != 3 || state.TotalSteps != 6 || state.Detail != "llama.cpp" ||
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
	// Context changes rebuild only the owned native engine, with the same model
	// and fixed bridge URLs exposed to clients.
	result, err := a.local.setContext(m.ID, 16384)
	if err != nil || result["activeContextWindow"] != 16384 {
		t.Fatal("native context replacement failed", err, result)
	}
	if err = a.local.checkModel(ctx, m.ID); err != nil {
		t.Fatal("tool calling failed after context replacement", err)
	}
	// A settings write failure after replacement must restore the prior engine.
	contextFile := a.path("local-context.json")
	previousData, err := os.ReadFile(contextFile)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(contextFile); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(contextFile, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = a.local.setContext(m.ID, 8192); err == nil || a.local.contextSize(m.ID) != 16384 {
		t.Fatal("failed persistence did not restore runtime context", err)
	}
	if err = os.Remove(contextFile); err != nil {
		t.Fatal(err)
	}
	if err = writeAtomic(contextFile, previousData); err != nil {
		t.Fatal(err)
	}
	if err = a.local.checkModel(ctx, m.ID); err != nil {
		t.Fatal("previous runtime could not resume after rollback", err)
	}
	t.Log("Idle context replacement and persistence-failure rollback passed")
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
