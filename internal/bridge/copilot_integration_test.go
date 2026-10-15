package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInstalledCopilotProviderStreamingAndTools(t *testing.T) {
	sdk, cli := os.Getenv("KOBASHI_TEST_COPILOT_SDK"), os.Getenv("KOBASHI_TEST_COPILOT_EXE")
	if sdk == "" || cli == "" {
		t.Skip("set KOBASHI_TEST_COPILOT_SDK and KOBASHI_TEST_COPILOT_EXE for isolated Copilot runtime checks")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	for _, wire := range []string{"responses", "completions"} {
		t.Run(wire, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				var body object
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				path := "/v1/responses"
				if wire == "completions" {
					path = "/chat/completions"
				}
				if r.URL.Path != path || body["model"] != "gpt-5.4" ||
					r.Header.Get("Authorization") != "Bearer synthetic-account-b" {
					t.Error("runtime request lost model, protocol, or account isolation", r.URL.Path, body["model"])
				}
				if call == 2 {
					field, kind, value := "input", "type", "function_call_output"
					if wire == "completions" {
						field, kind, value = "messages", "role", "tool"
					}
					found := false
					for _, item := range objectList(body, field) {
						found = found || (item[kind] == value && strings.Contains(string(jsonBytes(item)), "echo-value"))
					}
					if !found {
						t.Error("runtime did not replay the tool result")
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				usage := object{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15,
					"input_tokens_details": object{"cached_tokens": 0}, "output_tokens_details": object{"reasoning_tokens": 0}}
				if wire == "completions" {
					delta, finish := object{"role": "assistant", "content": "KOBASHI_OK"}, "stop"
					if call == 1 {
						delta = object{"role": "assistant", "tool_calls": []any{object{"index": 0, "id": "call-1", "type": "function",
							"function": object{"name": "echo_probe", "arguments": `{"value":"echo-value"}`}}}}
						finish = "tool_calls"
					}
					for _, choice := range []object{{"index": 0, "delta": delta, "finish_reason": nil},
						{"index": 0, "delta": object{}, "finish_reason": finish}} {
						fmt.Fprintf(w, "data: %s\n\n", jsonBytes(object{"id": "chat-1", "object": "chat.completion.chunk",
							"created": 1, "model": "gpt-5.4", "choices": []any{choice}}))
						w.(http.Flusher).Flush()
					}
					fmt.Fprint(w, "data: [DONE]\n\n")
					return
				}
				sequence := 0
				send := func(event object) {
					event["sequence_number"] = sequence
					sequence++
					fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], jsonBytes(event))
					w.(http.Flusher).Flush()
				}
				result := object{"id": fmt.Sprintf("resp-%d", call), "object": "response", "created_at": 1,
					"model": "gpt-5.4", "status": "in_progress", "output": []any{}}
				send(object{"type": "response.created", "response": clone(result)})
				var item object
				if call == 1 {
					item = object{"id": "fc-1", "type": "function_call", "call_id": "call-1",
						"name": "echo_probe", "arguments": "", "status": "in_progress"}
					send(object{"type": "response.output_item.added", "output_index": 0, "item": clone(item)})
					arguments := `{"value":"echo-value"}`
					send(object{"type": "response.function_call_arguments.delta", "item_id": "fc-1", "output_index": 0, "delta": arguments})
					send(object{"type": "response.function_call_arguments.done", "item_id": "fc-1", "output_index": 0, "arguments": arguments})
					item["arguments"] = arguments
				} else {
					part := object{"type": "output_text", "text": "", "annotations": []any{}}
					item = object{"id": "msg-1", "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}
					send(object{"type": "response.output_item.added", "output_index": 0, "item": clone(item)})
					send(object{"type": "response.content_part.added", "item_id": "msg-1", "output_index": 0, "content_index": 0, "part": clone(part)})
					send(object{"type": "response.output_text.delta", "item_id": "msg-1", "output_index": 0, "content_index": 0, "delta": "KOBASHI_OK"})
					send(object{"type": "response.output_text.done", "item_id": "msg-1", "output_index": 0, "content_index": 0, "text": "KOBASHI_OK"})
					part["text"] = "KOBASHI_OK"
					send(object{"type": "response.content_part.done", "item_id": "msg-1", "output_index": 0, "content_index": 0, "part": part})
					item["content"] = []any{part}
				}
				item["status"] = "completed"
				send(object{"type": "response.output_item.done", "output_index": 0, "item": item})
				result["status"], result["output"], result["usage"] = "completed", []any{item}, usage
				send(object{"type": "response.completed", "response": result})
			}))
			defer upstream.Close()
			a := testApp(t, func(r *http.Request) (*http.Response, error) {
				clone := r.Clone(r.Context())
				clone.URL.Scheme, clone.URL.Host = "http", strings.TrimPrefix(upstream.URL, "http://")
				return http.DefaultTransport.RoundTrip(clone)
			})
			a.state = session{Token: "synthetic-github-account-b", Copilot: true}
			a.bearer, a.expiry = "synthetic-account-b", float64(time.Now().Add(time.Hour).Unix())
			provider := httptest.NewServer(http.HandlerFunc(a.copilotApp))
			defer provider.Close()
			home := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			script := `
import {pathToFileURL} from "node:url";
import {join} from "node:path";
const {CopilotClient} = await import(pathToFileURL(join(process.env.KOBASHI_TEST_COPILOT_SDK, "index.js")));
const client = new CopilotClient({
  connection: {kind: "stdio", path: process.env.KOBASHI_TEST_COPILOT_EXE,
    args: ["--no-auto-update", "--disable-builtin-mcps", "--no-custom-instructions", "--no-remote-export"]},
  mode: "empty", baseDirectory: process.env.COPILOT_HOME, workingDirectory: process.cwd(),
  useLoggedInUser: false, logLevel: "error"
});
let toolCalls = 0;
try {
  const session = await client.createSession({
    model: "gpt-5.4", streaming: true,
    provider: {type: "openai", baseUrl: process.env.COPILOT_PROVIDER_BASE_URL,
      apiKey: "PROXY_MANAGED", wireApi: process.env.COPILOT_PROVIDER_WIRE_API},
    systemMessage: {mode: "replace", content: "Use echo_probe once, then reply with the result."},
    availableTools: ["echo_probe"],
    tools: [{name: "echo_probe", description: "Return a synthetic value.",
      parameters: {type: "object", properties: {value: {type: "string"}}, required: ["value"]},
      handler: async (args) => {toolCalls++; return {value: args.value};}}],
    onPermissionRequest: async () => ({kind: "approved"})
  });
  const result = await session.sendAndWait({prompt: "Call echo_probe with echo-value."}, 30000);
  if (toolCalls !== 1 || !result?.data.content.includes("KOBASHI_OK")) {
    throw new Error("Runtime did not complete the streaming tool-call roundtrip (" + toolCalls + " tool calls): " + JSON.stringify(result));
  }
  console.log("KOBASHI_RUNTIME_OK");
} finally {
  await client.stop();
}`
			command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script)
			command.Dir = home
			for _, entry := range os.Environ() {
				key := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
				if strings.HasPrefix(key, "COPILOT_") || strings.HasPrefix(key, "GITHUB_") ||
					strings.HasPrefix(key, "GH_") || strings.HasPrefix(key, "OTEL_") ||
					contains([]string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"}, key) {
					continue
				}
				command.Env = append(command.Env, entry)
			}
			command.Env = append(command.Env, "HOME="+home, "USERPROFILE="+home, "APPDATA="+home, "LOCALAPPDATA="+home,
				"COPILOT_HOME="+home, "COPILOT_OFFLINE=true", "COPILOT_PROVIDER_BASE_URL="+provider.URL+"/v1",
				"COPILOT_PROVIDER_TYPE=openai", "COPILOT_PROVIDER_WIRE_API="+wire, "COPILOT_PROVIDER_API_KEY=PROXY_MANAGED",
				"COPILOT_MODEL=gpt-5.4", "HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "NO_PROXY=127.0.0.1,localhost")
			output, err := command.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "KOBASHI_RUNTIME_OK") || calls.Load() != 2 {
				t.Fatalf("Copilot runtime roundtrip failed (%d calls): %v\n%s", calls.Load(), err, output)
			}
		})
	}
}
