package bridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalRecommendation(t *testing.T) {
	for _, c := range []struct {
		total, available int64
		want             string
	}{{16, 10, "qwen3-4b"}, {32, 28, "qwen3-coder-30b"}, {48, 40, "qwen3-coder-30b"}, {64, 56, "qwen3-coder-30b"}, {64, 10, "qwen3-4b"}, {16, 4, ""}} {
		if got := recommendedLocalModel(c.total<<30, c.available<<30); got != c.want {
			t.Fatalf("RAM %d/%d: %s != %s", c.available, c.total, got, c.want)
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
	if err := a.local.runtimeConfig("qwen3-4b", 12345, "private"); err != nil {
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
