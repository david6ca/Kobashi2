package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func accountTestTransport(r *http.Request) (*http.Response, error) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "token ")
	switch r.URL.Path {
	case "/user":
		if token == "rejected" {
			return response(401, object{}), nil
		}
		return response(200, object{"login": strings.TrimSuffix(token, "-secret")}), nil
	case "/copilot_internal/v2/token":
		return response(200, object{"token": "bearer-" + token, "expires_at": 9999999999}), nil
	}
	return nil, errors.New("unexpected upstream request: " + r.URL.Path)
}

func readSavedAccounts(t *testing.T, a *App) session {
	t.Helper()
	data, err := os.ReadFile(a.path("session.json"))
	var saved session
	if err != nil || json.Unmarshal(data, &saved) != nil {
		t.Fatal("could not read saved accounts", err)
	}
	if info, err := os.Stat(a.path("session.json")); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("credentials must remain private", err)
	}
	return saved
}

func TestAccountsAddSwitchAndReload(t *testing.T) {
	a := testApp(t, accountTestTransport)
	a.state = session{Token: "first-secret", Username: "first", Method: "token", Copilot: true}
	a.bearer, a.expiry = "old-bearer", 9999999999
	a.usage, a.rawModels, a.lastEffort = object{"old": true}, []object{{"id": "old"}}, object{"old": true}
	result := controlCall(a, "/api/token-login", object{"token": "second-secret"})
	if result.Code != 200 || !strings.Contains(result.Body.String(), `"ok":true`) {
		t.Fatal("add failed", result.Body.String())
	}
	s := a.snapshot()
	if s.Username != "second" || s.Codex || s.Claude || !s.Copilot || len(s.Accounts) != 2 {
		t.Fatal("adding lost accounts or changed client preferences", publicAccounts(s))
	}
	if a.usage != nil || a.rawModels != nil || a.lastEffort != nil || a.bearer != "bearer-second-secret" {
		t.Fatal("account caches were not invalidated")
	}
	if saved := readSavedAccounts(t, a); !reflect.DeepEqual(s, saved) {
		t.Fatal("active account was not persisted")
	}
	result = controlCall(a, "/api/accounts/switch", object{"username": "FIRST"})
	if result.Code != 200 || a.snapshot().Username != "first" || !a.snapshot().Copilot {
		t.Fatal("switch did not preserve bridge enablement", result.Body.String())
	}
	status := controlCall(a, "/api/status", nil)
	for _, secret := range []string{"first-secret", "second-secret", "bearer-", "github_token"} {
		if strings.Contains(status.Body.String(), secret) {
			t.Fatal("status exposed account credentials")
		}
	}
	var value object
	_ = json.Unmarshal(status.Body.Bytes(), &value)
	accounts := arr(value["accounts"])
	if len(accounts) != 2 || obj(accounts[0])["selected"] != true || obj(accounts[1])["selected"] != false {
		t.Fatal("incorrect account selection", status.Body.String())
	}
	reloaded, err := New(a.options)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Stop()
	reloaded.initialize()
	if !reflect.DeepEqual(reloaded.snapshot(), a.snapshot()) {
		t.Fatal("relaunch lost the account list or selection")
	}
	// Signing in to the same GitHub username refreshes its credential in place.
	result = controlCall(a, "/api/token-login", object{"token": "FIRST-secret"})
	if result.Code != 200 || len(a.snapshot().Accounts) != 2 || a.snapshot().Accounts[0].Token != "FIRST-secret" {
		t.Fatal("reauthentication duplicated an account", result.Body.String())
	}
}

func TestAccountsAddDuringInferencePreservesRunningAccount(t *testing.T) {
	entered, finished := make(chan struct{}), make(chan struct{})
	reader, writer := io.Pipe()
	a := testApp(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.githubcopilot.com" {
			if r.Header.Get("Authorization") != "Bearer existing-bearer" {
				t.Error("running inference used a different account")
			}
			close(entered)
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}, nil
		}
		return accountTestTransport(r)
	})
	a.state = session{Token: "first-secret", Username: "first", Method: "token", Codex: true}
	a.bearer, a.expiry = "existing-bearer", 9999999999
	a.usage, a.lastEffort = object{"preserve": true}, object{"effort": "high"}
	a.models.sessionStarted = true
	before := a.snapshot()
	generation := a.accountGeneration
	w := httptest.NewRecorder()
	go func() {
		defer close(finished)
		a.codex(w, httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-test","input":"test","stream":true}`)))
	}()
	t.Cleanup(func() {
		_ = writer.Close()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("inference did not finish")
		}
	})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("inference did not start")
	}
	result := controlCall(a, "/api/token-login", object{"token": "second-secret"})
	if result.Code != 200 || !strings.Contains(result.Body.String(), `"ok":true`) {
		t.Fatal("adding an account was blocked by inference", result.Body.String())
	}
	after := a.snapshot()
	before.Accounts = after.Accounts
	if !reflect.DeepEqual(before, after) || len(after.Accounts) != 2 || a.accountGeneration != generation || a.bearer != "existing-bearer" || a.usage["preserve"] != true || a.lastEffort["effort"] != "high" {
		t.Fatal("adding changed the selected account or its inference state", publicAccounts(after))
	}
	if saved := readSavedAccounts(t, a); !reflect.DeepEqual(saved, after) {
		t.Fatal("added account was not persisted")
	}
	// An explicit switch must still wait instead of changing the account used
	// by requests that are already running. Adding does not queue a switch.
	result = controlCall(a, "/api/accounts/switch", object{"username": "second"})
	if result.Code == 200 || a.snapshot().Username != "first" {
		t.Fatal("switched while inference was running", result.Body.String())
	}
	select {
	case <-finished:
		t.Fatal("adding interrupted inference")
	default:
	}
	_, err := writer.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"output\":[]}}\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("inference did not complete")
	}
	if w.Code != 200 || !strings.Contains(w.Body.String(), "response.completed") || a.snapshot().Username != "first" {
		t.Fatal("original inference or account selection changed", w.Code)
	}
	result = controlCall(a, "/api/accounts/switch", object{"username": "second"})
	if result.Code != 200 || a.snapshot().Username != "second" {
		t.Fatal("saved account could not be selected after inference", result.Body.String())
	}
}

func TestAccountsBusyDeviceLoginRedeemsOnlyOnce(t *testing.T) {
	redeemed := 0
	a := testApp(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "github.com" {
			redeemed++
			return response(200, object{"access_token": "second-secret"}), nil
		}
		return accountTestTransport(r)
	})
	a.state = session{Token: "first-secret", Username: "first", Method: "token"}
	a.routes.RLock()
	result, err := a.pollLogin(context.Background(), "device-code")
	a.routes.RUnlock()
	if err != nil || result["ok"] != true || redeemed != 1 || a.pendingLogin.token != "" {
		t.Fatal("device sign-in did not finish while busy", result, err, redeemed)
	}
	if s := readSavedAccounts(t, a); s.Username != "first" || len(s.Accounts) != 2 || s.Accounts[1].Method != "device" {
		t.Fatal("device sign-in replaced the current connection", publicAccounts(s))
	}
}

func TestAccountsBusyAddPreservesLocalProviderAndSetup(t *testing.T) {
	a := testApp(t, accountTestTransport)
	a.state = session{LocalModel: "qwen3-4b", LocalModels: []string{"qwen3-4b"}, Codex: true}
	canceled := false
	a.local.cancel = func() { canceled = true }
	a.routes.RLock()
	result := controlCall(a, "/api/token-login", object{"token": "second-secret"})
	a.routes.RUnlock()
	if result.Code != 200 || canceled || a.snapshot().LocalModel != "qwen3-4b" || !a.snapshot().Codex {
		t.Fatal("adding GitHub changed the local provider or preparation", result.Body.String())
	}
	if s := readSavedAccounts(t, a); len(s.Accounts) != 1 || s.Accounts[0].Username != "second" || s.LocalModel != "qwen3-4b" {
		t.Fatal("GitHub account was not saved alongside the local provider")
	}
}

func TestAccountsBusyReauthorizationAndSaveFailure(t *testing.T) {
	for _, failSave := range []bool{false, true} {
		t.Run(map[bool]string{false: "reauthorize", true: "save failure"}[failSave], func(t *testing.T) {
			a := testApp(t, accountTestTransport)
			a.state = session{Token: "first-secret", Username: "first", Method: "token", Codex: true}
			a.bearer, a.expiry = "old-bearer", 9999999999
			before := a.snapshot()
			if failSave {
				if err := os.MkdirAll(a.path("session.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			a.routes.RLock()
			result := controlCall(a, "/api/token-login", object{"token": "FIRST-secret"})
			a.routes.RUnlock()
			if failSave {
				if result.Code == 200 || !reflect.DeepEqual(before, a.snapshot()) || a.bearer != "old-bearer" {
					t.Fatal("failed save changed the live credentials", result.Body.String())
				}
				return
			}
			s := readSavedAccounts(t, a)
			if result.Code != 200 || len(s.Accounts) != 1 || s.Token != "FIRST-secret" || s.Accounts[0].Token != s.Token || !s.Codex || a.bearer != "bearer-FIRST-secret" {
				t.Fatal("same-identity reauthorization duplicated or lost the credential", result.Body.String())
			}
		})
	}
}

func TestAccountsFailedSwitchAndSaveKeepConnection(t *testing.T) {
	for _, failure := range []string{"invalid", "network", "save"} {
		t.Run(failure, func(t *testing.T) {
			a := testApp(t, accountTestTransport)
			a.state = session{Token: "first-secret", Username: "first", Method: "token",
				Accounts: []account{{Token: "rejected", Username: "second", Method: "token"}}}
			a.bearer = "existing-bearer"
			before := a.snapshot()
			if failure == "network" {
				a.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
					return nil, errors.New("offline")
				})
			}
			path, body := "/api/accounts/switch", object{"username": "second"}
			if failure == "save" {
				if err := os.MkdirAll(a.path("session.json"), 0700); err != nil {
					t.Fatal(err)
				}
				path, body = "/api/token-login", object{"token": "second-secret"}
			}
			result := controlCall(a, path, body)
			if strings.Contains(result.Body.String(), `"ok":true`) || !reflect.DeepEqual(before, a.snapshot()) || a.bearer != "existing-bearer" {
				t.Fatal("failed account action replaced the working account", result.Body.String())
			}
		})
	}
}

func TestAccountsMigrateDisconnectAndDelete(t *testing.T) {
	a := testApp(t, accountTestTransport)
	legacy := filepath.Join(a.options.Home, ".codex", "ccb-session.json")
	if err := writeJSON(legacy, session{Token: "first-secret", Username: "first", Method: "token"}); err != nil {
		t.Fatal(err)
	}
	a.initialize()
	if len(a.snapshot().Accounts) != 1 {
		t.Fatal("legacy login was not included in the account list")
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy login was duplicated", err)
	}
	if result := controlCall(a, "/api/disconnect", object{}); result.Code != 200 || a.snapshot().active() {
		t.Fatal("disconnect failed", result.Body.String())
	}
	saved := readSavedAccounts(t, a)
	if saved.active() || len(saved.Accounts) != 1 {
		t.Fatal("disconnect erased the saved account")
	}
	reloaded, err := New(a.options)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Stop()
	reloaded.initialize()
	if reloaded.snapshot().active() || len(reloaded.snapshot().Accounts) != 1 {
		t.Fatal("disconnected saved accounts did not survive startup")
	}
	if result := controlCall(a, "/api/accounts/delete", object{"username": "first"}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	if len(readSavedAccounts(t, a).Accounts) != 0 {
		t.Fatal("deleted credential remains saved")
	}
}

func TestAccountsDeleteInactiveAndActive(t *testing.T) {
	a := testApp(t, accountTestTransport)
	a.state = session{Token: "first-secret", Username: "first", Method: "token",
		Accounts: []account{{Token: "second-secret", Username: "second", Method: "token"}}}
	if result := controlCall(a, "/api/accounts/delete", object{"username": "second"}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	if s := a.snapshot(); s.Username != "first" || !s.active() || len(s.Accounts) != 1 {
		t.Fatal("deleting an inactive account changed the connection")
	}
	if result := controlCall(a, "/api/accounts/delete", object{"username": "first"}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	if a.snapshot().active() || len(a.snapshot().Accounts) != 0 {
		t.Fatal("active account was not removed")
	}
	if _, err := os.Stat(a.path("session.json")); !os.IsNotExist(err) {
		t.Fatal("last credential remains on disk", err)
	}
}

func TestAccountsDeleteActiveHonorsCopilotRestoration(t *testing.T) {
	a := testApp(t, nil)
	a.state = session{Token: "first-secret", Username: "first"}
	mock := mockCopilotSetup(t, a)
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock.mu.Lock()
	mock.selected.Running = 1
	mock.mu.Unlock()
	result := controlCall(a, "/api/accounts/delete", object{"username": "first"})
	if result.Code == 200 || !a.snapshot().active() || !a.snapshot().Copilot {
		t.Fatal("deletion interrupted a running dependent session", result.Body.String())
	}
	if !readSavedAccounts(t, a).active() {
		t.Fatal("unsafe deletion removed the active credential")
	}
}

func TestAccountsEndpointsRejectUnauthorizedAndUnknownAccounts(t *testing.T) {
	a := testApp(t, nil)
	for _, path := range []string{"/api/accounts/delete", "/api/accounts/switch"} {
		r := httptest.NewRequest("POST", path, strings.NewReader(`{"username":"first"}`))
		w := httptest.NewRecorder()
		a.control(w, r)
		if w.Code != 401 {
			t.Fatal("account action accepted an unauthenticated caller")
		}
		for _, body := range []object{{}, {"username": "missing"}} {
			if result := controlCall(a, path, body); result.Code == 200 {
				t.Fatal("invalid account action silently succeeded", path)
			}
		}
	}
}
