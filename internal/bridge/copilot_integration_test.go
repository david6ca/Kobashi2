package bridge

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/coder/websocket/wsjson"
)

func TestInstalledCopilotLiveChat(t *testing.T) {
	id := os.Getenv("KOBASHI_TEST_COPILOT_LIVE_SESSION")
	if id == "" {
		t.Skip("set KOBASHI_TEST_COPILOT_LIVE_SESSION to a new disposable idle chat to verify real app inference")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(home, ".copilot", "data.db")
	selectedData, err := copilotSessionData(db)
	if err != nil {
		t.Fatal(err)
	}
	var selected copilotSelection
	if len(selectedData) > 0 && json.Unmarshal(selectedData, &selected) != nil {
		t.Fatal("invalid app selection")
	}
	if selected.ID == id {
		t.Fatal("the live check must not target the user's selected chat")
	}
	read := func(query string) ([]copilotSelection, error) {
		data, err := copilotQuery(db, query)
		if err != nil {
			return nil, err
		}
		return decodeCopilotSessions(data)
	}
	sessions, err := read(copilotBridgeSessionsSQL)
	if err != nil || len(sessions) != 0 {
		t.Fatal("the live check requires no existing sessions dependent on Kobashi", err)
	}
	testSessions := func() ([]copilotSelection, error) {
		sessions, err := read(copilotLocalSessionsSQL)
		if err != nil {
			return nil, err
		}
		for _, session := range sessions {
			if session.ID == id {
				return []copilotSelection{session}, nil
			}
		}
		return nil, errors.New("the disposable test chat is unavailable")
	}
	readTest := func() (copilotSelection, error) {
		sessions, err := testSessions()
		if err != nil {
			return copilotSelection{}, err
		}
		return sessions[0], nil
	}
	original, err := readTest()
	if err != nil || original.ready() != nil || original.customProvider() || original.ProviderID == copilotProviderID {
		t.Fatal("the live check requires a new idle local chat", err)
	}
	if os.Getenv("KOBASHI_TEST_COPILOT_DORMANT") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		control, err := connectCopilotControl(ctx, home)
		if err != nil {
			t.Fatal(err)
		}
		status, err := control.call(ctx, object{"type": "probe_session_liveness", "session_id": id, "recover": false}, "session_liveness_result")
		control.conn.CloseNow()
		if err != nil || status["status"] != "idle_without_runtime" || status["is_running"] != false {
			t.Fatal("the cold-start check requires a dormant disposable chat", err)
		}
		t.Log("LIVE_CHAT_DORMANT")
	}
	eventPath := filepath.Join(home, ".copilot", "session-state", id, "events.jsonl")
	history, err := os.ReadFile(eventPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(history), "\n") {
		var event object
		if json.Unmarshal([]byte(line), &event) == nil && event["type"] == "user.message" {
			t.Fatal("the live check must not send another message to an existing conversation")
		}
	}
	data, err := os.ReadFile(filepath.Join(home, ".kobashi", "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	var account session
	if json.Unmarshal(data, &account) != nil || account.Token == "" {
		t.Fatal("Kobashi must already have an authorized GitHub account")
	}
	client := newHTTPClient()
	a := testApp(t, client.Do)
	a.state = session{Token: account.Token, Method: account.Method}
	a.copilotSession, a.copilotLocalSessions = readTest, testSessions
	a.copilotSessions = func() ([]copilotSelection, error) {
		sessions, err := testSessions()
		if err == nil && sessions[0].ProviderID != copilotProviderID {
			sessions = nil
		}
		return sessions, err
	}
	for _, name := range []string{"ws.port", "ws.token"} {
		data, err := os.ReadFile(filepath.Join(home, ".copilot", "run", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := writeAtomic(filepath.Join(a.options.Home, ".copilot", "run", name), data); err != nil {
			t.Fatal(err)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:18925")
	if err != nil {
		t.Fatal("the live check never replaces a running Kobashi listener", err)
	}
	var requests atomic.Int32
	var sentAt, firstRequest atomic.Int64
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			requests.Add(1)
			if sentAt.Load() != 0 {
				firstRequest.CompareAndSwap(0, time.Now().UnixNano())
			}
		}
		a.copilotApp(w, r)
	}), ReadHeaderTimeout: 10 * time.Second}
	a.servers = append(a.servers, server)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Error(err)
		}
	}()
	t.Cleanup(func() {
		if err := a.restoreCopilot(); err != nil {
			t.Error("could not restore the disposable chat's original selection", err)
		} else {
			current, err := readTest()
			if err != nil || !sameCopilotChoice(original, current) {
				t.Error("the disposable chat's original selection was not restored", err)
			} else {
				t.Log("LIVE_CHAT_RESTORED")
			}
		}
	})
	if _, err := a.enableCopilot(); err != nil {
		t.Fatal(err)
	}
	current, err := readTest()
	connectionDeadline := time.Now().Add(30 * time.Second)
	for err == nil && current.ProviderID != copilotProviderID && time.Now().Before(connectionDeadline) {
		if err := a.syncCopilot(); err != nil {
			t.Fatal("automatic session routing failed", err)
		}
		time.Sleep(100 * time.Millisecond)
		current, err = readTest()
	}
	if err != nil || current.ProviderID != copilotProviderID {
		t.Fatal("the disposable chat was not connected before its first message", err)
	}
	t.Log("LIVE_CHAT_READY", id)
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Minute)
	defer cancel()
	control, err := connectCopilotControl(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer control.conn.CloseNow()
	baseline, err := os.ReadFile(eventPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	requestID := rand.Text()
	sentAt.Store(time.Now().UnixNano())
	if err := wsjson.Write(ctx, control.conn, object{"type": "send_message", "session_id": id,
		"prompt": "hi", "initial_prompt": true, "display_prompt": "hi", "canvas_context": []any{}, "request_id": requestID}); err != nil {
		t.Fatal("could not send the disposable chat's first message", err)
	}
	readErrors := make(chan error, 1)
	go func() {
		for {
			var message object
			if err := wsjson.Read(ctx, control.conn, &message); err != nil {
				readErrors <- err
				return
			}
			if message["request_id"] == requestID && message["type"] == "error" {
				readErrors <- errors.New("the app rejected the disposable chat's first message")
				return
			}
		}
	}()
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case err := <-readErrors:
			t.Fatal("the live app control connection failed", err)
		default:
		}
		data, err := os.ReadFile(eventPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if len(data) < len(baseline) {
			t.Fatal("the disposable chat's event history was replaced during the check")
		}
		replied, idle := false, false
		for _, line := range strings.Split(string(data[len(baseline):]), "\n") {
			var event object
			if json.Unmarshal([]byte(line), &event) != nil {
				continue
			}
			if event["type"] == "assistant.message" && str(nested(event, "data", "content")) != "" {
				replied = true
			}
			if event["type"] == "assistant.turn_end" {
				idle = true
			}
		}
		current, err = readTest()
		if replied && idle && requests.Load() > 0 && err == nil && current.ready() == nil {
			if firstRequest.Load() == 0 {
				t.Fatal("the reply did not use Kobashi after the first message was sent")
			}
			dispatch := time.Duration(firstRequest.Load() - sentAt.Load())
			t.Logf("LIVE_CHAT_DISPATCH %s", dispatch)
			if dispatch > 15*time.Second {
				t.Errorf("the first message waited %s before inference; want at most 15s", dispatch)
			}
			t.Logf("LIVE_CHAT_REPLIED through Kobashi (%d inference requests)", requests.Load())
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("the disposable chat did not reply and become idle (%d inference requests)", requests.Load())
}

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
const wrapper = process.env.KOBASHI_TEST_COPILOT_WRAPPER;
const args = ["--no-auto-update", "--disable-builtin-mcps", "--no-custom-instructions", "--no-remote-export"];
const client = new CopilotClient({
  connection: {kind: "stdio", path: wrapper || process.env.KOBASHI_TEST_COPILOT_EXE,
    args: wrapper ? ["copilot", "--copilot-path", process.env.KOBASHI_TEST_COPILOT_EXE, ...args] : args},
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
  let timer;
  try {
    await Promise.race([
      session.rpc.mode.get(),
      new Promise((_, reject) => {timer = setTimeout(() => reject(new Error("Session activity RPC did not respond")), 5000);})
    ]);
  } finally {
    clearTimeout(timer);
  }
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
