package bridge

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func connectionHistoryFixture() object {
	return object{"model": "gpt-test", "stream": true, "store": false,
		"reasoning": object{"effort": "high"}, "tools": []any{object{"type": "function", "name": "read"}},
		"input": []any{
			object{"type": "message", "role": "user", "content": "Keep my context"},
			object{"type": "reasoning", "id": "rs_old", "encrypted_content": "old-account-opaque-data", "summary": []any{}},
			object{"type": "message", "id": "msg_old", "role": "assistant", "content": []any{object{"type": "output_text", "text": "Checking"}}},
			object{"type": "function_call", "id": "fc_old", "call_id": "call_1", "name": "read", "arguments": "{}"},
			object{"type": "function_call_output", "call_id": "call_1", "output": "file contents"},
			object{"type": "custom_tool_call", "id": "ct_old", "call_id": "call_2", "name": "edit", "input": "patch"},
			object{"type": "custom_tool_call_output", "call_id": "call_2", "output": "done"},
		}}
}

func TestConnectionHistoryRecovery(t *testing.T) {
	for _, client := range []string{"Codex", "Copilot"} {
		for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
			for _, rejectAgain := range []bool{false, true} {
				t.Run(client+path+map[bool]string{false: "/recovered", true: "/terminal"}[rejectAgain], func(t *testing.T) {
					original := connectionHistoryFixture()
					originalBytes := jsonBytes(original)
					want := clone(original)
					items := arr(want["input"])
					want["input"] = append(items[:1], items[2:]...)
					for _, i := range []int{1, 2, 4} {
						delete(obj(arr(want["input"])[i]), "id")
					}
					calls := 0
					const rejection = `{"error":{"message":"input item does not belong to this connection"}}`
					a := testApp(t, func(r *http.Request) (*http.Response, error) {
						if r.URL.Host != "api.githubcopilot.com" {
							t.Fatal("history rejection triggered token refresh")
						}
						if r.URL.Path != path || r.Header.Get("Authorization") != "Bearer valid" {
							t.Fatal("route or token changed")
						}
						calls++
						data, _ := io.ReadAll(r.Body)
						var actual object
						if err := json.Unmarshal(data, &actual); err != nil {
							t.Fatal(err)
						}
						expected := originalBytes
						if calls > 1 {
							expected = jsonBytes(want)
						}
						if !bytes.Equal(jsonBytes(actual), expected) {
							t.Fatalf("history/context changed unexpectedly: %s", data)
						}
						if calls == 1 || rejectAgain {
							result := response(401, nil)
							result.Body = io.NopCloser(strings.NewReader(rejection))
							result.Header.Set("X-Request-Id", "upstream-history-error")
							return result, nil
						}
						return successfulProbe(), nil
					})
					a.state = session{Token: "account", Codex: true, Copilot: true}
					a.bearer, a.expiry = "valid", float64(time.Now().Add(time.Hour).Unix())
					a.models.sessionStarted = true
					w := httptest.NewRecorder()
					r := httptest.NewRequest("POST", path, bytes.NewReader(originalBytes))
					if client == "Codex" {
						a.codex(w, r)
					} else {
						r.Header.Set("Authorization", "Bearer PROXY_MANAGED")
						a.copilotApp(w, r)
					}
					if calls != 2 || a.bearer != "valid" {
						t.Fatalf("calls=%d, credential invalidated=%t", calls, a.bearer != "valid")
					}
					if rejectAgain {
						if w.Code != 401 || w.Body.String() != rejection || w.Header().Get("X-Request-Id") != "upstream-history-error" {
							t.Fatal("terminal rejection not preserved", w.Code, w.Body.String())
						}
					} else if w.Code != 200 || !strings.Contains(w.Body.String(), "response.completed") {
						t.Fatal(w.Code, w.Body.String())
					}
				})
			}
		}
	}
}

func TestInputConnectionMismatchClassification(t *testing.T) {
	for _, body := range []string{
		"input item does not belong to this connection\n",
		`{"error":{"message":"input item ID does not belong to this connection"}}`,
		`{"message":"input item does not belong to this connection"}`,
		`{"error":"input item does not belong to this connection"}`,
	} {
		for _, status := range []int{400, 401} {
			if !inputConnectionMismatch(status, []byte(body)) || authFailure(status, []byte(body)) {
				t.Fatal("history mistaken for credentials", status, body)
			}
		}
	}
	for _, body := range []string{"Unauthorized", `{"error":{"message":"Invalid API key"}}`,
		`{"error":{"message":"Unauthorized","metadata":"input item does not belong to this connection"}}`,
		"<html>input item does not belong to this connection</html>"} {
		if inputConnectionMismatch(401, []byte(body)) || !authFailure(401, []byte(body)) {
			t.Fatal("unrelated 401 classified as history", body)
		}
	}
	if inputConnectionMismatch(403, []byte("input item does not belong to this connection")) {
		t.Fatal("unexpected status matched")
	}
}

func TestConnectionHistoryPreservesUnrecoverableContext(t *testing.T) {
	for _, kind := range []string{"previous_response_id", "item_reference", "compaction", "no_history", "reasoning_only"} {
		t.Run(kind, func(t *testing.T) {
			body := connectionHistoryFixture()
			switch kind {
			case "previous_response_id":
				body[kind] = "resp_old"
			case "item_reference", "compaction":
				body["input"] = append(arr(body["input"]), object{"type": kind, "id": "old", "encrypted_content": "only-copy"})
			case "no_history":
				body["input"] = []any{object{"role": "user", "content": "hello"}}
			case "reasoning_only":
				body["input"] = []any{object{"type": "reasoning", "encrypted_content": "opaque"}}
			}
			before := jsonBytes(body)
			next, changed := repairConnectionHistory(body)
			if changed || !bytes.Equal(before, jsonBytes(body)) || !bytes.Equal(before, jsonBytes(next)) {
				t.Fatal("unrecoverable context was changed")
			}
		})
	}
	original := connectionHistoryFixture()
	before := jsonBytes(original)
	next, changed := repairConnectionHistory(original)
	if !changed || !bytes.Equal(before, jsonBytes(original)) {
		t.Fatal("original request mutated")
	}
	if _, changed = repairConnectionHistory(next); changed {
		t.Fatal("repair was not bounded")
	}
}
