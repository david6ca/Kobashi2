package bridge

import (
	"bufio"
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
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func copilotRequest(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer PROXY_MANAGED")
	return r
}

func TestCopilotTogglePersistsWithoutChangingClientLogin(t *testing.T) {
	a := testApp(t, func(*http.Request) (*http.Response, error) {
		t.Error("Copilot setup must not make network requests")
		return nil, errors.New("unexpected upstream call")
	})
	mockCopilotSetup(t, a)
	a.options.ReadEnvironment = func(string) (*string, error) {
		t.Fatal("Copilot setup read the user environment")
		return nil, nil
	}
	a.options.WriteEnvironment = func(string, *string) error {
		t.Fatal("Copilot setup changed the user environment")
		return nil
	}
	if a.snapshot().Copilot || a.CopilotURL() != "http://127.0.0.1:18925/v1" {
		t.Fatal("Copilot must be opt-in with an independent endpoint")
	}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 400 {
		t.Fatal("disconnected bridge was enabled")
	}
	loginPath := filepath.Join(a.options.Home, ".copilot", "config.json")
	login := []byte(`{"logged_in_users":[{"login":"account-a"}],"user_setting":"keep"}`)
	if err := writeAtomic(loginPath, login); err != nil {
		t.Fatal(err)
	}
	a.state = session{Token: "github-account-b", Username: "account-b"}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	data, err := os.ReadFile(a.path("session.json"))
	var saved session
	if err != nil || json.Unmarshal(data, &saved) != nil || !saved.Copilot || saved.Codex || saved.Claude {
		t.Fatal("Copilot was not independently persisted", err)
	}
	reloaded, err := New(a.options)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.initialize()
	if !reloaded.snapshot().Copilot {
		t.Error("Copilot must preserve its enabled state across launches")
	}
	if err := reloaded.Stop(); err != nil {
		t.Fatal(err)
	}
	status := controlCall(a, "/api/status", nil)
	var value object
	if status.Code != 200 || json.Unmarshal(status.Body.Bytes(), &value) != nil ||
		value["copilotEnabled"] != true || value["copilotURL"] != a.CopilotURL() ||
		strings.Contains(status.Body.String(), a.state.Token) {
		t.Fatal("invalid or credential-bearing Copilot status", status.Body.String())
	}
	for _, operation := range []string{"/api/toggle-copilot", "/api/toggle-copilot", "/api/disconnect"} {
		if result := controlCall(a, operation, object{}); result.Code != 200 {
			t.Fatal(result.Body.String())
		}
	}
	current, err := os.ReadFile(loginPath)
	if err != nil || string(current) != string(login) {
		t.Fatal("Copilot account A login was modified", err)
	}
	for _, path := range []string{a.options.CodexHome, filepath.Join(a.options.Home, ".claude"), a.path("environment.json")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("Copilot setup touched another client", path, err)
		}
	}
	w := httptest.NewRecorder()
	a.copilotApp(w, copilotRequest("POST", "/v1/responses", `{}`))
	if w.Code != 401 {
		t.Fatal("disconnected Copilot bridge still accepts inference")
	}
}

func TestCopilotStartupPreservesEnablementWithoutChangingAppSessions(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			a := testApp(t, func(*http.Request) (*http.Response, error) {
				t.Fatal("startup must not contact Copilot")
				return nil, errors.New("unexpected upstream request")
			})
			a.copilotSession = func() (copilotSelection, error) {
				t.Fatal("startup must not read or change the selected app session")
				return copilotSelection{}, errors.New("unexpected app access")
			}
			a.copilotSessions = func() ([]copilotSelection, error) {
				t.Fatal("startup must not restore or interrupt app sessions")
				return nil, errors.New("unexpected app access")
			}
			saved := object{"github_token": "saved-login", "username": "example", "authMethod": "token",
				"codexEnabled": false, "claudeEnabled": false, "copilotEnabled": enabled,
				"futureSetting": "preserve"}
			if err := writeJSON(a.path("session.json"), saved); err != nil {
				t.Fatal(err)
			}
			backup := []byte(`{"saved-session":{"id":"saved-session","model":"original","providerId":"original-provider"}}`)
			if err := writeAtomic(a.path("copilot-restore.json"), backup); err != nil {
				t.Fatal(err)
			}
			a.initialize()
			state := a.snapshot()
			if state.Copilot != enabled || state.Token != "saved-login" || state.Username != "example" ||
				state.Method != "token" || state.Codex || state.Claude {
				t.Fatal("startup changed login or the saved bridge preferences", state.Copilot, state.Codex, state.Claude)
			}
			data, err := os.ReadFile(a.path("session.json"))
			var persisted object
			if err != nil || json.Unmarshal(data, &persisted) != nil {
				t.Fatal("could not read startup state", err)
			}
			if !reflect.DeepEqual(persisted, saved) {
				t.Fatal("startup changed unrelated session preferences")
			}
			if data, err := os.ReadFile(a.path("copilot-restore.json")); err != nil || !bytes.Equal(data, backup) {
				t.Fatal("startup lost original provider selections", err)
			}
			var status object
			result := controlCall(a, "/api/status", nil)
			if json.Unmarshal(result.Body.Bytes(), &status) != nil || status["copilotEnabled"] != enabled {
				t.Fatal("startup status did not preserve Copilot enablement")
			}
			if !enabled {
				result = httptest.NewRecorder()
				a.copilotApp(result, copilotRequest("POST", "/v1/responses", `{}`))
				if result.Code != http.StatusUnauthorized {
					t.Fatal("disabled startup still allows inference", result.Code)
				}
			}
		})
	}
}

func TestCopilotToggleRollsBackWhenSessionCannotBeSaved(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	if err := os.MkdirAll(a.path("session.json"), 0700); err != nil {
		t.Fatal(err)
	}
	result := controlCall(a, "/api/toggle-copilot", object{})
	if result.Code != 400 || a.snapshot().Copilot {
		t.Fatal("unsaved Copilot state was enabled", result.Body.String())
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	for _, command := range mock.requests {
		if command["type"] != "list_model_providers" && command["type"] != "list_provider_models" {
			t.Fatal("failed persistence mutated app configuration", command["type"])
		}
	}
}

func TestCopilotModelCatalogDoesNotRequireCodex(t *testing.T) {
	var calls atomic.Int32
	a := testApp(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Host != "api.githubcopilot.com" || r.URL.Path != "/models" ||
			r.Header.Get("Authorization") != "Bearer account-b-inference" {
			t.Error("model catalog did not use account B")
		}
		return response(200, object{"data": []any{
			object{"id": "gpt-test", "vendor": "OpenAI", "supported_endpoints": []any{"/responses"}},
			object{"id": "claude-test", "vendor": "Anthropic", "supported_endpoints": []any{"/chat/completions"}},
		}}), nil
	})
	a.state = session{Token: "github-account-b", Copilot: true}
	a.bearer, a.expiry = "account-b-inference", float64(time.Now().Add(time.Hour).Unix())
	a.options.NativeCatalog = func(context.Context, string) ([]object, error) {
		t.Fatal("Copilot catalog requested the Codex native catalog")
		return nil, nil
	}
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		a.copilotApp(w, copilotRequest("GET", "/v1/models", ""))
		var catalog object
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &catalog) != nil || catalog["object"] != "list" {
			t.Fatal("invalid OpenAI-compatible catalog", w.Body.String())
		}
		models := objectList(catalog, "data")
		if len(models) != 2 || models[0]["id"] != "gpt-test" || models[1]["id"] != "claude-test" ||
			models[0]["object"] != "model" || models[0]["owned_by"] != "OpenAI" || models[0]["created"] != float64(0) {
			t.Fatal("account models were filtered or changed", catalog)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("catalog lookup made inference probes or ignored discovery cache", calls.Load())
	}
}

func TestCopilotInferenceUsesOnlyBridgeAccountAndPreservesPayloads(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/responses/compact", "/v1/chat/completions"} {
		t.Run(path, func(t *testing.T) {
			payload := `{"model":"selected-model","stream":true,"input":[{"type":"agent_message","encrypted_content":"opaque"}],"tools":[{"type":"function","name":"read_file","parameters":{"type":"object"}}],"messages":[{"role":"tool","tool_call_id":"call-1","content":"original"}]}`
			reply := `{"id":"reply-1","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-2","type":"function","function":{"name":"read_file","arguments":"{}"}}]}}]}`
			calls := 0
			a := testApp(t, func(r *http.Request) (*http.Response, error) {
				calls++
				expected := path
				if path == "/v1/chat/completions" {
					expected = "/chat/completions"
				}
				if r.URL.Host != "api.githubcopilot.com" || r.URL.Path != expected || r.URL.RawQuery != "test=keep" ||
					r.Header.Get("Authorization") != "Bearer account-b-inference" ||
					r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("Cookie") != "" ||
					r.Header.Get("X-Github-Token") != "" {
					t.Error("inference leaked client credentials or changed its route")
				}
				data, err := io.ReadAll(r.Body)
				if err != nil || string(data) != payload {
					t.Error("model, history, tools, or encrypted content changed", err)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(reply))}, nil
			})
			a.state = session{Token: "github-account-b", Copilot: true}
			a.bearer, a.expiry = "account-b-inference", float64(time.Now().Add(time.Hour).Unix())
			r := copilotRequest("POST", path+"?test=keep", payload)
			r.Header.Set("Cookie", "github-account-a-cookie")
			r.Header.Set("X-Github-Token", "github-account-a-token")
			w := httptest.NewRecorder()
			a.copilotApp(w, r)
			if w.Code != 200 || w.Body.String() != reply || calls != 1 {
				t.Fatal("inference or tool-call response was not preserved", w.Code, w.Body.String(), calls)
			}
		})
	}
}

func TestCopilotEndpointIsolationAndDisabledState(t *testing.T) {
	a := testApp(t, func(*http.Request) (*http.Response, error) {
		t.Error("blocked request reached the upstream")
		return nil, errors.New("unexpected upstream call")
	})
	a.state = session{Token: "account-b", Copilot: true}
	for _, tc := range []struct {
		method, path, key, origin string
		code                      int
	}{
		{"GET", "/v1/models", "", "", 401},
		{"GET", "/v1/models", "account-a-token", "", 401},
		{"GET", "/v1/models", "PROXY_MANAGED", "https://example.invalid", 403},
		{"POST", "/api/quit", "PROXY_MANAGED", "", 404},
		{"POST", "/v1/arbitrary", "PROXY_MANAGED", "", 404},
		{"DELETE", "/v1/models", "PROXY_MANAGED", "", 404},
	} {
		r := copilotRequest(tc.method, tc.path, "")
		r.Header.Set("Authorization", "Bearer "+tc.key)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		a.copilotApp(w, r)
		if w.Code != tc.code {
			t.Fatal(tc.path, w.Code, w.Body.String())
		}
	}
	a.state.Copilot = false
	w := httptest.NewRecorder()
	a.copilotApp(w, copilotRequest("POST", "/v1/responses", `{}`))
	if w.Code != 401 {
		t.Fatal("disabled bridge accepted requests")
	}
}

func TestCopilotUpstreamErrorsAreNotHiddenOrReplayed(t *testing.T) {
	for _, status := range []int{403, 429, 500} {
		calls := 0
		body := `{"error":{"code":"upstream_limit","message":"original upstream error"}}`
		a := testApp(t, func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}, "Retry-After": {"17"}},
				Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		a.state = session{Token: "account-b", Copilot: true}
		a.bearer, a.expiry = "account-b-inference", float64(time.Now().Add(time.Hour).Unix())
		w := httptest.NewRecorder()
		a.copilotApp(w, copilotRequest("POST", "/v1/responses", `{"model":"selected-model","input":"test"}`))
		if w.Code != status || w.Body.String() != body || calls != 1 {
			t.Fatal("upstream failure changed or was replayed", status, w.Code, w.Body.String(), calls)
		}
	}
}

func TestCopilotStreamingFlushesBeforeUpstreamCompletes(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/chat/completions"} {
		t.Run(path, func(t *testing.T) {
			chunk := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{}\"}}]}}]}\n\n"
			if path == "/v1/responses" {
				chunk = "event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"item-1\",\"type\":\"function_call\",\"call_id\":\"call-1\",\"name\":\"read_file\",\"arguments\":\"{}\"}}\n\n"
			}
			var once sync.Once
			release := make(chan struct{})
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			go func() {
				io.WriteString(writer, chunk)
				<-release
				io.WriteString(writer, "data: [DONE]\n\n")
				writer.Close()
			}()
			a := testApp(t, func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}, nil
			})
			a.state = session{Token: "account-b", Copilot: true}
			a.bearer, a.expiry = "account-b-inference", float64(time.Now().Add(time.Hour).Unix())
			server := httptest.NewServer(http.HandlerFunc(a.copilotApp))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+path, strings.NewReader(`{"model":"selected-model","stream":true}`))
			req.Header.Set("Authorization", "Bearer PROXY_MANAGED")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				unblock()
				t.Fatal("stream was buffered until completion", err)
			}
			defer res.Body.Close()
			scanner := bufio.NewScanner(res.Body)
			var first strings.Builder
			for scanner.Scan() {
				first.WriteString(scanner.Text() + "\n")
				if scanner.Text() == "" {
					break
				}
			}
			unblock()
			if first.String() != chunk {
				t.Fatal("streamed tool call changed or was not flushed", first.String(), scanner.Err())
			}
			found := false
			for scanner.Scan() {
				found = found || scanner.Text() == "data: [DONE]"
			}
			if err := scanner.Err(); err != nil || !found {
				t.Fatal("stream did not finish", err)
			}
		})
	}
}

/*
func TestRemoteCopilotOnlyClientUsesServerAccount(t *testing.T) {
	var calls atomic.Int32
	serverApp, server := remoteFixture(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer copilot-server-only" {
			t.Error("Copilot remote inference did not use the Server account")
		}
		if r.URL.Path == "/models" {
			return response(200, object{"data": []any{object{"id": "gpt-remote", "vendor": "OpenAI"}}}), nil
		}
		if r.URL.Path != "/v1/responses" {
			t.Error("unexpected upstream path", r.URL.Path)
		}
		return response(200, object{"id": "server-response"}), nil
	})
	serverApp.state.Codex, serverApp.state.Claude, serverApp.state.Copilot = false, false, true
	client := testApp(t, func(*http.Request) (*http.Response, error) {
		t.Error("Client contacted GitHub directly")
		return nil, errors.New("unexpected upstream call")
	})
	if err := client.connectRemote(context.Background(), server.URL, testServerToken); err != nil {
		t.Fatal(err)
	}
	if state := client.snapshot(); !state.Copilot || state.Codex || state.Claude || state.Token != "" {
		t.Fatal("Copilot-only Server could not be used independently")
	}
	for _, req := range []*http.Request{
		copilotRequest("GET", "/v1/models", ""),
		copilotRequest("POST", "/v1/responses", `{"model":"gpt-remote","input":"test"}`),
	} {
		w := httptest.NewRecorder()
		client.copilotApp(w, req)
		if w.Code != 200 || (!strings.Contains(w.Body.String(), "gpt-remote") && !strings.Contains(w.Body.String(), "server-response")) {
			t.Fatal("Copilot remote request failed", w.Code, w.Body.String())
		}
	}
	serverApp.mu.Lock()
	serverApp.state.Copilot = false
	serverApp.mu.Unlock()
	w := httptest.NewRecorder()
	client.copilotApp(w, copilotRequest("POST", "/v1/responses", `{}`))
	if w.Code != 401 || calls.Load() != 2 {
		t.Fatal("Server Copilot toggle did not stop remote access", w.Code, calls.Load())
	}
}
*/
