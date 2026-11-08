package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const localPatchFixture = "*** Begin Patch\n*** Add File: saved.txt\n+Qwen \"writes\" files \\ safely\n+你好\n*** End Patch\n"

func localCustomFixture() object {
	return object{"model": "qwen3-4b", "tools": []any{
		object{"type": "custom", "name": "apply_patch", "description": "Edit files",
			"format": object{"type": "grammar", "syntax": "lark", "definition": "start: patch"}},
		object{"type": "function", "name": "shell", "parameters": object{"type": "object"}}},
		"input": []any{object{"type": "message", "role": "user", "content": "Save a file."}}}
}

func localFunctionFixture(name, namespace, arguments string) object {
	suffix := "patch"
	if name != "apply_patch" || namespace != "" {
		suffix = namespace + "_" + name
	}
	item := object{"type": "function_call", "name": name, "id": "fc_" + suffix, "call_id": "call_" + suffix,
		"status": "completed", "arguments": arguments}
	if namespace != "" {
		item["namespace"] = namespace
	}
	return item
}

func TestLocalCustomToolsRequestAndHistory(t *testing.T) {
	body := localCustomFixture()
	body["tool_choice"] = object{"type": "custom", "name": "apply_patch"}
	body["input"] = []any{
		object{"type": "custom_tool_call", "id": "ctc_patch", "call_id": "call_patch", "name": "apply_patch", "input": localPatchFixture},
		object{"type": "custom_tool_call_output", "call_id": "call_patch", "output": "File saved."},
		object{"type": "function_call", "name": "shell", "arguments": `{"command":"pwd"}`, "call_id": "call_shell"},
	}
	custom, err := prepareLocalCustomTools(body)
	if err != nil || !custom[[2]string{"", "apply_patch"}] || len(custom) != 1 {
		t.Fatal(custom, err)
	}
	tool := obj(arr(body["tools"])[0])
	if tool["type"] != "function" || tool["format"] != nil ||
		nested(tool, "parameters", "properties", "input", "type") != "string" ||
		!strings.Contains(str(tool["description"]), "start: patch") ||
		nested(body, "tool_choice", "type") != "function" {
		t.Fatal("freeform definition was not made visible to the model", body)
	}
	input := arr(body["input"])
	call := obj(input[0])
	raw, err := localCustomInput(str(call["arguments"]))
	if err != nil || raw != localPatchFixture || call["input"] != nil ||
		call["type"] != "function_call" || call["id"] != "ctc_patch" || call["call_id"] != "call_patch" {
		t.Fatal("history lost raw tool input or identity", input, err)
	}
	if obj(input[1])["type"] != "function_call_output" || obj(input[1])["output"] != "File saved." ||
		obj(input[2])["arguments"] != `{"command":"pwd"}` {
		t.Fatal("tool outputs or ordinary functions changed", input)
	}
}

func TestLocalCustomToolsNamespacesAndCollisions(t *testing.T) {
	body := localCustomFixture()
	body["tools"] = append(arr(body["tools"]), object{"type": "namespace", "name": "editor", "tools": []any{
		object{"type": "custom", "name": "apply_patch"}, object{"type": "function", "name": "shell"}}})
	custom, err := prepareLocalCustomTools(body)
	if err != nil || len(custom) != 2 {
		t.Fatal(custom, err)
	}
	args := string(jsonBytes(object{"input": localPatchFixture}))
	result := object{"output": []any{
		localFunctionFixture("apply_patch", "", args),
		localFunctionFixture("apply_patch", "editor", args),
		localFunctionFixture("apply_patch", "other", `{"command":"pwd"}`),
		localFunctionFixture("shell", "", `{"command":"pwd"}`)}}
	if err := custom.restoreResponse(result); err != nil {
		t.Fatal(err)
	}
	items := arr(result["output"])
	if obj(items[0])["type"] != "custom_tool_call" || obj(items[1])["type"] != "custom_tool_call" ||
		obj(items[1])["namespace"] != "editor" || obj(items[2])["type"] != "function_call" || obj(items[3])["type"] != "function_call" {
		t.Fatal("tool namespace or ordinary functions were lost", result)
	}
	for _, reverse := range []bool{false, true} {
		body := localCustomFixture()
		tools := arr(body["tools"])
		conflict := object{"type": "function", "name": "apply_patch"}
		if reverse {
			body["tools"] = append([]any{conflict}, tools...)
		} else {
			body["tools"] = append(tools, conflict)
		}
		if _, err := prepareLocalCustomTools(body); err == nil {
			t.Fatal("ambiguous tool name accepted")
		}
	}
}

func localCustomSSE(events ...object) string {
	var out strings.Builder
	for _, event := range events {
		out.WriteString("event: " + str(event["type"]) + "\r\ndata: " + string(jsonBytes(event)) + "\r\n\r\n")
	}
	return out.String()
}

func localCustomStreamFixture(arguments string, argumentsDone bool) string {
	arguments = strings.ReplaceAll(arguments, "你好", `\u4f60\u597d`)
	added := localFunctionFixture("apply_patch", "", "")
	added["status"] = "in_progress"
	events := []object{
		{"type": "response.created", "response": object{"id": "resp_patch", "output": []any{}}},
		{"type": "response.output_item.added", "output_index": 1, "item": added},
		{"type": "response.output_item.added", "output_index": 2, "item": localFunctionFixture("shell", "", "")},
	}
	// Split the ASCII JSON wrapper at every byte, including escape sequences.
	for i := 0; i < len(arguments); i++ {
		events = append(events, object{"type": "response.function_call_arguments.delta", "item_id": "fc_patch", "output_index": 1,
			"delta": arguments[i : i+1]})
	}

	events = append(events, object{"type": "response.function_call_arguments.delta", "output_index": 2, "delta": `{"command":"pwd"}`})
	if argumentsDone {
		events = append(events, object{"type": "response.function_call_arguments.done", "item_id": "fc_patch", "output_index": 1, "arguments": arguments})
	}
	events = append(events,
		object{"type": "response.output_item.done", "output_index": 1, "item": localFunctionFixture("apply_patch", "", arguments)},
		object{"type": "response.completed", "response": object{"id": "resp_patch", "status": "completed", "output": []any{
			localFunctionFixture("shell", "", `{"command":"pwd"}`), localFunctionFixture("apply_patch", "", arguments)}}})
	return ": keepalive\r\n\r\n" + localCustomSSE(events...)
}

func TestInstalledCodexLocalFreeformFileWrite(t *testing.T) {
	checkInstalledCodexLocalFreeform(t, false)
}

func TestInstalledCodexLocalFreeformFailure(t *testing.T) {
	checkInstalledCodexLocalFreeform(t, true)
}

func checkInstalledCodexLocalFreeform(t *testing.T, invalid bool) {
	t.Helper()
	executable := os.Getenv("KOBASHI_TEST_CODEX_EXE")
	if executable == "" {
		t.Skip("set KOBASHI_TEST_CODEX_EXE to verify file writes with an isolated Codex CLI")
	}
	a := localTestApp(t)
	a.state = session{LocalModel: "qwen3-4b", Method: "local", Codex: true}
	var requests atomic.Int32
	var history atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body object
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		var events []object
		switch requests.Add(1) {
		case 1:
			found := false
			for _, entry := range arr(body["tools"]) {
				tool := obj(entry)
				if tool["name"] == "apply_patch" && tool["type"] == "function" &&
					nested(tool, "parameters", "properties", "input", "type") == "string" {
					found = true
				}
			}
			if !found {
				t.Error("Codex's freeform patch tool was not visible to the local runtime")
			}
			args := string(jsonBytes(object{"input": localPatchFixture}))
			if invalid {
				args = "{}"
			}
			added := localFunctionFixture("apply_patch", "", "")
			added["status"] = "in_progress"
			events = []object{
				{"type": "response.created", "response": object{"id": "resp_patch", "object": "response", "status": "in_progress", "output": []any{}}},
				{"type": "response.output_item.added", "output_index": 0, "item": added},
				{"type": "response.function_call_arguments.delta", "output_index": 0, "item_id": "fc_patch", "delta": args},
				{"type": "response.function_call_arguments.done", "output_index": 0, "item_id": "fc_patch", "arguments": args},
				{"type": "response.output_item.done", "output_index": 0, "item": localFunctionFixture("apply_patch", "", args)},
				{"type": "response.completed", "response": object{"id": "resp_patch", "object": "response", "status": "completed",
					"output": []any{localFunctionFixture("apply_patch", "", args)}, "usage": object{"input_tokens": 10, "output_tokens": 20, "total_tokens": 30}}},
			}
		case 2:
			call, result := false, false
			for _, entry := range arr(body["input"]) {
				item := obj(entry)
				if item["type"] == "function_call" && item["name"] == "apply_patch" && item["call_id"] == "call_patch" {
					raw, err := localCustomInput(str(item["arguments"]))
					call = err == nil && raw == localPatchFixture
				}
				if item["type"] == "function_call_output" && item["call_id"] == "call_patch" {
					result = strings.Contains(str(item["output"]), "saved.txt")
				}
			}
			history.Store(call && result)
			message := object{"type": "message", "id": "msg_saved", "role": "assistant", "status": "completed",
				"content": []any{object{"type": "output_text", "text": "Saved.", "annotations": []any{}}}}
			events = []object{
				{"type": "response.created", "response": object{"id": "resp_saved", "status": "in_progress", "output": []any{}}},
				{"type": "response.output_item.added", "output_index": 0, "item": message},
				{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "item_id": "msg_saved", "delta": "Saved."},
				{"type": "response.output_text.done", "output_index": 0, "content_index": 0, "item_id": "msg_saved", "text": "Saved."},
				{"type": "response.output_item.done", "output_index": 0, "item": message},
				{"type": "response.completed", "response": object{"id": "resp_saved", "status": "completed", "output": []any{message}}},
			}
		default:
			http.Error(w, "isolated test exceeded two inference requests", 500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, localCustomSSE(events...))
	}))
	defer upstream.Close()
	a.local.endpoint, a.local.secret = upstream.URL, "local-secret"
	bridge := httptest.NewServer(http.HandlerFunc(a.codex))
	defer bridge.Close()
	if err := a.local.writeCatalog("qwen3-4b"); err != nil {
		t.Fatal(err)
	}
	home := a.options.CodexHome
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	port := bridge.Listener.Addr().(*net.TCPAddr).Port
	config := injectConfig("cli_auth_credentials_store = \"file\"\nmodel = \"qwen3-4b\"\napproval_policy = \"never\"\n",
		port, a.path("codex-model-catalog.json"))
	config += "stream_max_retries = 0\nrequest_max_retries = 0\n"
	for name, data := range map[string][]byte{"config.toml": []byte(config), "auth.json": syntheticChatGPTAuth()} {
		if err := os.WriteFile(filepath.Join(home, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "exec", "--skip-git-repo-check", "--sandbox", "workspace-write", "--json",
		"Use apply_patch to create saved.txt in this working directory.")
	cmd.Dir = workspace
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + a.options.Home, "CODEX_HOME=" + home,
		"TMPDIR=" + os.TempDir(), "HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1",
		"ALL_PROXY=http://127.0.0.1:1", "NO_PROXY=127.0.0.1,localhost"}
	output, err := cmd.CombinedOutput()
	if invalid {
		if _, statErr := os.Stat(filepath.Join(workspace, "saved.txt")); err == nil || !os.IsNotExist(statErr) ||
			!strings.Contains(string(output), "input string") || requests.Load() != 1 {
			t.Fatal("Codex did not display the adapter failure without executing a patch:", err, requests.Load(), string(output))
		}
		return
	}
	if err != nil {
		t.Fatal("isolated Codex failed:", err, string(output))
	}
	data, err := os.ReadFile(filepath.Join(workspace, "saved.txt"))
	if err != nil || string(data) != "Qwen \"writes\" files \\ safely\n你好\n" || !history.Load() || requests.Load() != 2 {
		t.Fatal("Codex did not execute the raw patch and return its result:", err, fmt.Sprintf("%q", data),
			"history:", history.Load(), "requests:", requests.Load(), string(output))
	}
}

func TestLocalCustomToolsStreaming(t *testing.T) {
	for _, done := range []bool{false, true} {
		t.Run(map[bool]string{false: "item_done_fallback", true: "arguments_done"}[done], func(t *testing.T) {
			custom, err := prepareLocalCustomTools(localCustomFixture())
			if err != nil {
				t.Fatal(err)
			}
			arguments := string(jsonBytes(object{"input": localPatchFixture}))
			var output bytes.Buffer
			if err := custom.restoreStream(strings.NewReader(localCustomStreamFixture(arguments, done)), &output); err != nil {
				t.Fatal(err)
			}
			count, deltas, dones := 0, 0, 0
			if err := readSSE(&output, func(e sseEvent) error {
				event, name := e.data()
				if event == nil {
					return nil
				}
				if name != str(event["type"]) || number(event["sequence_number"]) != float64(count) {
					t.Fatal("event type or sequence was corrupted", event, name, count)
				}
				count++
				switch event["type"] {
				case "response.custom_tool_call_input.delta":
					deltas++
					if event["delta"] != localPatchFixture || event["item_id"] != "fc_patch" || number(event["output_index"]) != 1 {
						t.Fatal("raw patch was corrupted", event)
					}
				case "response.custom_tool_call_input.done":
					dones++
					if event["input"] != localPatchFixture {
						t.Fatal(event)
					}
				case "response.function_call_arguments.delta":
					if number(event["output_index"]) != 2 || event["delta"] != `{"command":"pwd"}` {
						t.Fatal("wrapper JSON leaked to Codex or ordinary arguments changed", event)
					}
				case "response.output_item.done":
					if nested(event, "item", "type") != "custom_tool_call" || nested(event, "item", "input") != localPatchFixture {
						t.Fatal(event)
					}
				case "response.completed":
					items := arr(nested(event, "response", "output"))
					if obj(items[1])["input"] != localPatchFixture || obj(items[1])["arguments"] != nil ||
						obj(items[1])["call_id"] != "call_patch" || obj(items[0])["type"] != "function_call" {
						t.Fatal("final output was inconsistent", event)
					}
				}
				return nil
			}); err != nil || deltas != 1 || dones != 1 {
				t.Fatal("missing or duplicated custom tool input events", deltas, dones, err)
			}
		})
	}
}

func TestLocalCustomToolsRejectInvalidArguments(t *testing.T) {
	for _, arguments := range []string{"", "{", "null", `{"input":null}`, `{"input":123}`, `{"other":"patch"}`,
		`{"input":"patch"} {}`, `{"input":"patch","extra":true}`} {
		t.Run(arguments, func(t *testing.T) {
			custom, _ := prepareLocalCustomTools(localCustomFixture())
			result := object{"output": []any{localFunctionFixture("apply_patch", "", arguments)}}
			if err := custom.restoreResponse(result); err == nil {
				t.Fatal("invalid wrapper accepted")
			}

			var output bytes.Buffer
			if err := custom.restoreStream(strings.NewReader(localCustomStreamFixture(arguments, true)), &output); err == nil ||
				strings.Contains(output.String(), "response.completed") {
				t.Fatal("invalid patch appeared successful", err, output.String())
			}
		})
	}
	if input, err := localCustomInput(`{"input":""}`); err != nil || input != "" {
		t.Fatal("a valid empty string was rejected", err)
	}
	custom, _ := prepareLocalCustomTools(localCustomFixture())
	var output bytes.Buffer
	if err := custom.restoreStream(strings.NewReader(localCustomSSE(object{"type": "response.created"})), &output); err == nil {
		t.Fatal("disconnected upstream appeared successful")
	}
}

func TestLocalCustomToolsStreamingPreservesFailuresAndRejectsInconsistentCompletion(t *testing.T) {
	custom, _ := prepareLocalCustomTools(localCustomFixture())
	failed := object{"type": "response.failed", "response": object{"id": "resp_failure", "status": "failed",
		"error":  object{"code": "context_length_exceeded", "message": "Prompt is too long"},
		"output": []any{localFunctionFixture("apply_patch", "", "")}}}
	var output bytes.Buffer
	if err := custom.restoreStream(strings.NewReader(localCustomSSE(failed)), &output); err != nil ||
		!strings.Contains(output.String(), "Prompt is too long") {
		t.Fatal("original upstream failure was masked", err, output.String())
	}
	args := string(jsonBytes(object{"input": localPatchFixture}))
	for _, change := range []string{"input", "call_id", "namespace", "missing_done"} {
		t.Run(change, func(t *testing.T) {
			item := localFunctionFixture("apply_patch", "", args)
			switch change {
			case "input":
				item["arguments"] = `{"input":"different patch"}`
			case "call_id":
				item["call_id"] = "different_call"
			case "namespace":
				item["namespace"] = "different_namespace"
			}
			events := []object{{"type": "response.output_item.added", "output_index": 0,
				"item": localFunctionFixture("apply_patch", "", "")}}
			if change != "missing_done" {
				events = append(events, object{"type": "response.output_item.done", "output_index": 0,
					"item": localFunctionFixture("apply_patch", "", args)})
			}
			events = append(events, object{"type": "response.completed", "response": object{"status": "completed", "output": []any{item}}})
			var output bytes.Buffer
			if err := custom.restoreStream(strings.NewReader(localCustomSSE(events...)), &output); err == nil ||
				strings.Contains(output.String(), "response.completed") {
				t.Fatal("inconsistent stream appeared successful", err, output.String())
			}
		})
	}
}

func TestLocalCustomToolsStreamingCancellation(t *testing.T) {
	a := localTestApp(t)
	a.state = session{LocalModel: "qwen3-4b", Method: "local", Codex: true}
	closed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, localCustomSSE(object{"type": "response.created"}))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	defer upstream.Close()
	a.local.endpoint, a.local.secret = upstream.URL, "local-secret"
	bridge := httptest.NewServer(http.HandlerFunc(a.codex))
	defer bridge.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "POST", bridge.URL+"/v1/responses", bytes.NewReader(jsonBytes(localCustomFixture())))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer PROXY_MANAGED")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if _, err := io.ReadFull(response.Body, make([]byte, 1)); err != nil {
		t.Fatal("response did not stream before completion", err)
	}
	cancel()
	response.Body.Close()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled Codex stream left the local inference request open")
	}
}
func TestLocalCustomToolsNamespacedStreamAndBufferedArguments(t *testing.T) {
	body := localCustomFixture()
	body["tools"] = []any{object{"type": "namespace", "name": "editor", "tools": []any{object{"type": "custom", "name": "apply_patch"}}}}
	custom, err := prepareLocalCustomTools(body)
	if err != nil {
		t.Fatal(err)
	}
	args := string(jsonBytes(object{"input": localPatchFixture}))
	source := strings.ReplaceAll(localCustomStreamFixture(args, true), `"name":"apply_patch"`, `"name":"apply_patch","namespace":"editor"`)
	var buffered strings.Builder
	if err := readSSE(strings.NewReader(source), func(e sseEvent) error {
		event, _ := e.data()
		if event == nil {
			return nil
		}
		if event["type"] == "response.function_call_arguments.done" {
			delete(event, "arguments")
		}
		buffered.WriteString(localCustomSSE(event))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := custom.restoreStream(strings.NewReader(buffered.String()), &output); err != nil {
		t.Fatal("fragmented wrapper could not be decoded from buffered arguments", err)
	}
	found := false
	if err := readSSE(&output, func(e sseEvent) error {
		event, _ := e.data()
		if event["type"] == "response.completed" {
			item := obj(arr(nested(event, "response", "output"))[1])
			found = item["namespace"] == "editor" && item["type"] == "custom_tool_call" && item["input"] == localPatchFixture
		}
		return nil
	}); err != nil || !found {
		t.Fatal("namespaced stream lost its tool identity or raw input", err)
	}
}

func TestLocalCustomToolsProxy(t *testing.T) {
	for _, test := range []struct {
		name, contentType, response string
		status                      int
		wantStatus                  int
		want                        string
	}{
		{"json", "application/json", string(jsonBytes(object{"output": []any{localFunctionFixture("apply_patch", "",
			string(jsonBytes(object{"input": localPatchFixture})))}})), 200, 200, "custom_tool_call"},
		{"sse", "text/event-stream", localCustomStreamFixture(string(jsonBytes(object{"input": localPatchFixture})), true), 200, 200, "response.custom_tool_call_input.done"},
		{"invalid_json", "application/json", `{"output":[{"type":"function_call","name":"apply_patch","arguments":"{}"}]}`, 200, 502, "input string"},
		{"invalid_sse", "text/event-stream", localCustomStreamFixture(`{"bad":"patch"}`, true), 200, 200, "response.failed"},
		{"disconnect", "text/event-stream", localCustomSSE(object{"type": "response.created"}), 200, 200, "response.failed"},
		{"upstream_error", "application/json", `{"error":{"message":"Model refused"}}`, 429, 429, "Model refused"},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := localTestApp(t)
			a.state = session{LocalModel: "qwen3-4b", Method: "local", Codex: true}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body object
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				tool := obj(arr(body["tools"])[0])
				if tool["type"] != "function" || nested(tool, "parameters", "properties", "input", "type") != "string" {
					t.Error("runtime did not receive the wrapped function", body)
				}
				if r.Header.Get("Authorization") != "Bearer local-secret" || r.Header.Get("Cookie") != "" {
					t.Error("incorrect runtime credentials")
				}
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(test.status)
				io.WriteString(w, test.response)
			}))
			defer upstream.Close()
			a.local.endpoint, a.local.secret = upstream.URL, "local-secret"
			request := httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(jsonBytes(localCustomFixture())))
			request.Header.Set("Authorization", "Bearer PROXY_MANAGED")
			request.Header.Set("Cookie", "private")
			result := httptest.NewRecorder()
			a.codex(result, request)
			if result.Code != test.wantStatus || !strings.Contains(result.Body.String(), test.want) {
				t.Fatal(result.Code, result.Body.String())
			}
			if strings.Contains(test.name, "invalid") && strings.Contains(result.Body.String(), "response.completed") {
				t.Fatal("invalid tool output was presented as successful", result.Body.String())
			}
		})
	}
}
