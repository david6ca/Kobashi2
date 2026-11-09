package bridge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func forbiddenResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusForbidden,
		Header: http.Header{"Content-Type": {"text/plain"}},
		Body:   io.NopCloser(strings.NewReader(body))}
}

func TestForbiddenTokenRecovery(t *testing.T) {
	for _, client := range []string{"Codex", "Copilot"} {
		for _, tc := range []struct {
			name       string
			body       string
			rejectNew  bool
			wantStatus int
			refreshes  int
		}{
			{"stale token", "forbidden\n", false, 200, 1},
			{"case and whitespace", " Forbidden\r\n", false, 200, 1},
			{"fresh token also rejected", "forbidden\n", true, 403, 1},
			{"model access", `{"error":{"code":"model_access_denied","message":"forbidden"}}`, false, 403, 0},
			{"quota", `{"error":{"code":"quota_exceeded","message":"forbidden"}}`, false, 403, 0},
			{"account permission", `{"message":"forbidden"}`, false, 403, 0},
			{"other plain text", "forbidden by organization policy", false, 403, 0},
			{"HTML rejection", "<html>forbidden</html>", false, 403, 0},
		} {
			t.Run(client+"/"+tc.name, func(t *testing.T) {
				calls, refreshes := 0, 0
				const requestBody = `{"model":"gpt-test","input":"preserve this","stream":false}`
				a := testApp(t, func(r *http.Request) (*http.Response, error) {
					if r.URL.Host == "api.github.com" {
						refreshes++
						if r.URL.Path != "/copilot_internal/v2/token" {
							t.Fatalf("unexpected account request: %s", r.URL.Path)
						}
						return response(200, object{"token": "fresh", "expires_at": time.Now().Add(time.Hour).Unix()}), nil
					}
					calls++
					data, err := io.ReadAll(r.Body)
					if err != nil || string(data) != requestBody || r.URL.Path != "/v1/responses" {
						t.Fatalf("request changed during recovery: path=%s body=%s err=%v", r.URL.Path, data, err)
					}
					wantToken := "Bearer stale"
					if calls > 1 {
						wantToken = "Bearer fresh"
					}
					if r.Header.Get("Authorization") != wantToken {
						t.Fatal("request did not use the expected credential")
					}
					if calls == 1 || tc.rejectNew {
						return forbiddenResponse(tc.body), nil
					}
					return response(200, object{"id": "recovered", "status": "completed"}), nil
				})
				a.state = session{Token: "saved-account", Codex: true, Copilot: true}
				a.bearer, a.expiry = "stale", float64(time.Now().Add(time.Hour).Unix())
				a.models.sessionStarted = true
				w := httptest.NewRecorder()
				r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(requestBody))
				if client == "Codex" {
					a.codex(w, r)
				} else {
					r.Header.Set("Authorization", "Bearer PROXY_MANAGED")
					a.copilotApp(w, r)
				}
				if w.Code != tc.wantStatus || refreshes != tc.refreshes || calls != 1+tc.refreshes {
					t.Fatalf("status=%d refreshes=%d inference calls=%d", w.Code, refreshes, calls)
				}
				if w.Code == 403 && w.Body.String() != tc.body {
					t.Fatal("terminal upstream error was changed", w.Body.String())
				}
				if a.snapshot().Token != "saved-account" {
					t.Fatal("inference rejection changed the saved login")
				}
			})
		}
	}
}

func TestConcurrentForbiddenRecoveryKeepsFreshToken(t *testing.T) {
	const requests = 8
	var staleCalls, freshCalls, refreshes atomic.Int32
	allStale, refreshed := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a := testApp(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.github.com" {
			refreshes.Add(1)
			return response(200, object{"token": "fresh", "expires_at": time.Now().Add(time.Hour).Unix()}), nil
		}
		switch r.Header.Get("Authorization") {
		case "Bearer stale":
			n := staleCalls.Add(1)
			if n == requests {
				close(allStale)
			}
			select {
			case <-allStale:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			if n != 1 {
				// These failures arrive after another request has refreshed.
				select {
				case <-refreshed:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return forbiddenResponse("forbidden\n"), nil
		case "Bearer fresh":
			if freshCalls.Add(1) == 1 {
				close(refreshed)
			}
			return response(200, object{"status": "completed"}), nil
		default:
			return nil, fmt.Errorf("unexpected inference credential")
		}
	})
	a.state.Token = "saved-account"
	a.bearer, a.expiry = "stale", float64(time.Now().Add(time.Hour).Unix())
	results := make(chan error, requests)
	for range requests {
		go func() {
			r, err := a.copilot(ctx, "POST", "/v1/responses", []byte(`{"model":"gpt-test","input":"test"}`))
			if err == nil {
				_, err = readResponse(r, 1024)
				if err == nil && r.StatusCode != 200 {
					err = fmt.Errorf("unexpected response status %d", r.StatusCode)
				}
			}
			results <- err
		}()
	}
	for range requests {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	if refreshes.Load() != 1 || staleCalls.Load() != requests || freshCalls.Load() != requests {
		t.Fatalf("refreshes=%d stale calls=%d fresh calls=%d", refreshes.Load(), staleCalls.Load(), freshCalls.Load())
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.bearer != "fresh" {
		t.Fatal("a late rejection invalidated the fresh credential")
	}
}
