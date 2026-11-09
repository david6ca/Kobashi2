package bridge

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCodexResponsesRejectsUnsupportedMethodsLocally(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
		for _, method := range []string{"GET", "PUT", "DELETE"} {
			t.Run(method+path, func(t *testing.T) {
				logs := captureDiagnostics(t)
				a := testApp(t, func(*http.Request) (*http.Response, error) {
					t.Fatal("unsupported method reached upstream")
					return nil, nil
				})
				a.state = session{Token: "account", Codex: true}
				r := httptest.NewRequest(method, path+"?private-query", nil)
				websocket := method == "GET" && path == "/v1/responses"
				if websocket {
					r.Header.Set("Upgrade", "websocket")
				}
				r.Header.Set("Authorization", "Bearer private-token")
				w := httptest.NewRecorder()
				a.codex(w, r)
				if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "POST" {
					t.Fatal(w.Code, w.Header(), w.Body.String())
				}
				if !strings.Contains(logs.String(), fmt.Sprintf("websocket_upgrade=%t", websocket)) || strings.Contains(logs.String(), "private-") {
					t.Fatal("incorrect protocol diagnostics", logs.String())
				}
				// Disabled bridges must still return their authentication error.
				a.state.Codex = false
				w = httptest.NewRecorder()
				a.codex(w, r)
				if w.Code != http.StatusUnauthorized {
					t.Fatal("inactive bridge bypassed", w.Code)
				}
			})
		}
	}
}

func TestProxyRequestTooLargeDetails(t *testing.T) {
	for _, client := range []string{"Codex", "Copilot"} {
		for _, fixture := range []struct{ name, body, contentType, code string }{
			{"string", `{"error":"failed to parse request"}`, "application/json", "request_too_large"},
			{"structured", `{"error":{"code":"payload_limit","message":"failed to parse request","param":"input"},"request_id":"original"}`, "application/json", "payload_limit"},
			{"plain", "upstream size rejection", "text/plain", ""},
		} {
			t.Run(client+"/"+fixture.name, func(t *testing.T) {
				logs := captureDiagnostics(t)
				const payload = `{"model":"gpt-test","stream":true,"input":"private-request"}`
				calls := 0
				a := testApp(t, func(r *http.Request) (*http.Response, error) {
					calls++
					data, _ := io.ReadAll(r.Body)
					if string(data) != payload {
						t.Fatal("request data changed")
					}
					result := response(http.StatusRequestEntityTooLarge, nil)
					result.Body = io.NopCloser(strings.NewReader(fixture.body))
					result.Header.Set("Content-Type", fixture.contentType)
					result.Header.Set("Content-Length", fmt.Sprint(len(fixture.body)))
					result.Header.Set("X-Request-Id", "upstream-request")
					return result, nil
				})
				a.state = session{Token: "private-account", Codex: true, Copilot: true}
				a.bearer, a.expiry = "private-bearer", float64(time.Now().Add(time.Hour).Unix())
				a.models.sessionStarted = true
				r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(payload))
				w := httptest.NewRecorder()
				if client == "Copilot" {
					r.Header.Set("Authorization", "Bearer PROXY_MANAGED")
					a.copilotApp(w, r)
				} else {
					a.codex(w, r)
				}
				status := http.StatusRequestEntityTooLarge
				if client == "Codex" {
					status = http.StatusBadRequest
					if w.Header().Get("X-Kobashi-Upstream-Status") != "413" {
						t.Fatal("lost original upstream status")
					}
				}
				if calls != 1 || w.Code != status || w.Header().Get("X-Request-Id") != "upstream-request" || w.Header().Get("X-Should-Retry") != "false" {
					t.Fatal("status, correlation or retry behavior changed", calls, w.Code, w.Header())
				}
				if fixture.code != "" {
					var got object
					if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					if nested(got, "error", "code") != fixture.code || !strings.Contains(str(nested(got, "error", "message")), "Upstream: failed to parse request") || w.Header().Get("Content-Length") != "" {
						t.Fatal("lost upstream details or retained stale length", got, w.Header())
					}
					if fixture.name == "structured" && (nested(got, "error", "param") != "input" || got["request_id"] != "original") {
						t.Fatal("upstream metadata changed", got)
					}
				} else if w.Body.String() != fixture.body || w.Header().Get("Content-Type") != fixture.contentType {
					t.Fatal("unrecognized upstream body was replaced", w.Body.String())
				}
				for _, expected := range []string{"upstream_failure status=413", fmt.Sprintf("request_bytes=%d upstream_request_bytes=%d", len(payload), len(payload))} {
					if !strings.Contains(logs.String(), expected) {
						t.Fatal("missing size diagnostics", logs.String())
					}
				}
				if strings.Contains(logs.String(), "private-") {
					t.Fatal("request or credentials leaked into diagnostics")
				}
			})
		}
	}
}
