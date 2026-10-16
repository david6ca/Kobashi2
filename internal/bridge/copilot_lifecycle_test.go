package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCopilotEnableBeforeAppStarts(t *testing.T) {
	a := testApp(t, func(*http.Request) (*http.Response, error) {
		t.Error("enabling before the app starts must not contact GitHub")
		return nil, errors.New("unexpected request")
	})
	a.state = session{Token: "account-b"}
	result := controlCall(a, "/api/toggle-copilot", object{})
	if result.Code != 200 || !a.snapshot().Copilot {
		t.Fatal("an unavailable app prevented enablement", result.Body.String())
	}
	var saved session
	data, err := os.ReadFile(a.path("session.json"))
	if err != nil || json.Unmarshal(data, &saved) != nil || !saved.Copilot {
		t.Fatal("deferred enablement was not persisted", err)
	}
	if _, err := os.Stat(filepath.Join(a.options.Home, ".copilot")); !os.IsNotExist(err) {
		t.Fatal("deferred enablement wrote Copilot configuration", err)
	}
	if err := a.syncCopilot(); err != nil || !a.snapshot().Copilot {
		t.Fatal("waiting for the app changed enablement", err)
	}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 || a.snapshot().Copilot {
		t.Fatal("could not cancel deferred enablement", result.Body.String())
	}
}

func TestCopilotEnableWaitsForStaleControlPort(t *testing.T) {
	a := testApp(t, nil)
	a.state = session{Token: "account-b"}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"ws.port": port, "ws.token": "private-placeholder"} {
		if err := writeAtomic(filepath.Join(a.options.Home, ".copilot", "run", name),
			fmt.Appendf(nil, "%s\n%d\n", value, os.Getpid())); err != nil {
			t.Fatal(err)
		}
	}
	result := controlCall(a, "/api/toggle-copilot", object{})
	if result.Code != 200 || !a.snapshot().Copilot || strings.Contains(result.Body.String(), "private-placeholder") {
		t.Fatal("stale app files prevented deferred enablement or leaked authentication", result.Body.String())
	}
	if err := a.syncCopilot(); err != nil || !a.snapshot().Copilot {
		t.Fatal("stale app control port was not treated as waiting for startup", err)
	}
	if err := writeAtomic(filepath.Join(a.options.Home, ".copilot", "run", "ws.token"),
		[]byte("private-placeholder\n1\n")); err != nil {
		t.Fatal(err)
	}
	if err := a.syncCopilot(); err != nil || !a.snapshot().Copilot {
		t.Fatal("partially updated app control files disabled automatic connection", err)
	}
}

func TestCopilotAutomaticallyConnectsAfterAppStarts(t *testing.T) {
	a := testApp(t, nil)
	a.state = session{Token: "account-b"}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock := mockCopilotSetup(t, a)
	mock.selected.ProviderID = ""
	original := mock.selected
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if !a.snapshot().Copilot || mock.selected.ProviderID != copilotProviderID || mock.selected.Model != original.Model {
		t.Fatal("app startup still required selecting a session or toggling again")
	}
	var saved map[string]copilotSelection
	data, err := os.ReadFile(a.path("copilot-restore.json"))
	if err != nil || json.Unmarshal(data, &saved) != nil || saved[original.ID] != original {
		t.Fatal("deferred connection did not save the original selection", err)
	}
}

func TestCopilotConnectsDormantSessionBeforeFirstMessage(t *testing.T) {
	a := testApp(t, nil)
	a.state = session{Token: "account-b"}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock := mockCopilotSetup(t, a)
	mock.selected.ProviderID = ""
	mock.livenessStatus = "idle_without_runtime"
	original := mock.selected
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	connected := mock.selected.ProviderID == copilotProviderID
	mock.mu.Unlock()
	if !connected {
		t.Fatal("a dormant idle session was left on the original provider until after its first message")
	}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 || a.snapshot().Copilot {
		t.Fatal("a dormant session could not restore its original model", result.Body.String())
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.selected != original {
		t.Fatal("a dormant session's original selection was not restored")
	}
}

func TestCopilotWaitingForRuntimeDoesNotRewriteRegistration(t *testing.T) {
	a := testApp(t, nil)
	a.state = session{Token: "account-b"}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock := mockCopilotSetup(t, a)
	mock.selected.ProviderID = ""
	mock.livenessStatus = "unresponsive"
	for range 4 {
		if err := a.syncCopilot(); err != nil {
			t.Fatal(err)
		}
	}
	mock.mu.Lock()
	providers, models := 0, 0
	for _, command := range mock.requests {
		switch command["type"] {
		case "upsert_model_provider":
			providers++
		case "upsert_provider_model":
			models++
		case "set_session_model":
			t.Error("a runtime that is not ready was switched")
		}
	}
	mock.mu.Unlock()
	if providers != 1 || models != 1 {
		t.Fatalf("waiting rewrote the app's provider configuration: %d provider writes, %d model writes", providers, models)
	}
	mock.mu.Lock()
	probes := 0
	for _, command := range mock.requests {
		if command["type"] == "probe_session_liveness" {
			probes++
		}
	}
	mock.livenessStatus = "responsive"
	mock.mu.Unlock()
	if probes != 1 {
		t.Fatalf("an unready runtime was repeatedly probed without backoff: %d probes", probes)
	}
	a.copilotRouting.retryAt["selected-session"] = time.Now().Add(-time.Second)
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.selected.ProviderID != copilotProviderID {
		t.Fatal("the runtime did not connect after becoming ready")
	}
}

func TestCopilotPendingSessionDoesNotBlockOtherSessions(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalid-probe-%t", invalid), func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			created := mock.selected
			created.ProviderID = ""
			mock.selected = copilotSelection{}
			if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
				t.Fatal(result.Body.String())
			}
			mock.mu.Lock()
			created.ID = "a-unready"
			mock.sessions[created.ID] = created
			created.ID = "z-ready"
			mock.sessions[created.ID] = created
			mock.requests = nil
			mock.onLiveness = func(reply object) {
				if reply["session_id"] == "a-unready" {
					reply["status"] = "unresponsive"
					if invalid {
						delete(reply, "is_running")
					}
				}
			}
			mock.mu.Unlock()
			if err := a.syncCopilot(); (err != nil) != invalid {
				t.Fatal("runtime check failure was hidden or normal startup was reported as failure", err)
			}
			mock.mu.Lock()
			defer mock.mu.Unlock()
			if mock.sessions["a-unready"].ProviderID != "" || mock.sessions["z-ready"].ProviderID != copilotProviderID {
				t.Error("an unavailable runtime blocked another idle session or was switched unsafely")
			}
			for _, command := range mock.requests {
				if command["type"] == "upsert_model_provider" || command["type"] == "upsert_provider_model" {
					t.Error("connecting another session rewrote unchanged registration")
				}
			}
		})
	}
}

func TestCopilotDifferentModelsKeepStableProviderDefaults(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	created := mock.selected
	created.ProviderID = ""
	mock.selected = copilotSelection{}
	a.rawModels = append(a.rawModels, object{"id": "chat-model", "name": "Chat model",
		"supported_endpoints": []any{"/chat/completions"}})
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock.mu.Lock()
	created.ID = "a-responses"
	mock.sessions[created.ID] = created
	created.ID, created.Model = "b-completions", "chat-model"
	mock.sessions[created.ID] = created
	mock.livenessStatus = "unresponsive"
	mock.requests = nil
	mock.mu.Unlock()
	for range 2 {
		if err := a.syncCopilot(); err != nil {
			t.Fatal(err)
		}
		a.copilotRouting.retryAt = nil
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	for _, command := range mock.requests {
		if command["type"] == "upsert_model_provider" {
			t.Error("a model's wire protocol rewrote the shared provider's default")
		}
	}
	if len(mock.models) != 2 || mock.models[0]["wireApiOverride"] != "responses" || mock.models[1]["wireApiOverride"] != "completions" {
		t.Fatal("per-model protocol registration was not preserved", mock.models)
	}
}

func TestCopilotAutomaticallyConnectsNewBackgroundSessionWhenIdle(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	existing := mock.selected
	existing.ID, existing.ProviderID = "existing-background", ""
	mock.sessions[existing.ID] = existing
	mock.selected = copilotSelection{}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	created := existing
	created.ID, created.Running = "new-background", 1
	mock.mu.Lock()
	mock.sessions[created.ID] = created
	mock.mu.Unlock()
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	if mock.sessions[created.ID] != created || mock.sessions[existing.ID] != existing {
		t.Error("automatic connection changed a running or unrelated session")
	}
	created.Running = 0
	mock.sessions[created.ID] = created
	mock.mu.Unlock()
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	if mock.sessions[created.ID].ProviderID != copilotProviderID || mock.sessions[existing.ID] != existing {
		t.Error("new background session did not connect automatically when idle")
	}
	mock.mu.Unlock()
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 || a.snapshot().Copilot {
		t.Fatal("automatic background connection could not be restored", result.Body.String())
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.sessions[created.ID] != created || mock.sessions[existing.ID] != existing {
		t.Fatal("disabling changed an unrelated session or lost the new session's original settings")
	}
}

func TestCopilotPendingConnectionSurvivesTabChange(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	mock.selected.Running = 1
	original := mock.selected
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock.mu.Lock()
	mock.selected = copilotSelection{}
	original.Running = 0
	mock.sessions[original.ID] = original
	mock.mu.Unlock()
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.sessions[original.ID].ProviderID != copilotProviderID {
		t.Fatal("switching to Settings lost a pending connection")
	}
}

func TestCopilotAutomaticConnectionPreservesManualChoices(t *testing.T) {
	for _, pending := range []bool{false, true} {
		for _, field := range []string{"provider", "model", "reasoning", "context"} {
			t.Run(fmt.Sprintf("pending=%t/%s", pending, field), func(t *testing.T) {
				a := testApp(t, nil)
				mock := mockCopilotSetup(t, a)
				a.state = session{Token: "account-b"}
				if pending {
					mock.selected.Running = 1
				}
				if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
					t.Fatal(result.Body.String())
				}
				mock.mu.Lock()
				mock.selected.Running = 0
				switch field {
				case "provider":
					mock.selected.ProviderID = "user-provider"
				case "model":
					mock.selected.Model = "user-model"
				case "reasoning":
					mock.selected.ReasoningEffort = "low"
				case "context":
					mock.selected.ContextTier = "user-context"
				}
				changed := mock.selected
				mock.requests = nil
				mock.mu.Unlock()
				for range 2 {
					if err := a.syncCopilot(); err != nil {
						t.Fatal(err)
					}
				}
				mock.mu.Lock()
				defer mock.mu.Unlock()
				if mock.selected != changed {
					t.Fatal("automatic connection overrode a user's manual choice")
				}
				for _, command := range mock.requests {
					if command["type"] == "set_session_model" {
						t.Fatal("automatic connection attempted to override a manual choice")
					}
				}
			})
		}
	}
}

func TestCopilotAutomaticConnectionLeavesNewCustomProviderAlone(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	created := mock.selected
	created.ID, created.ProviderID = "custom-session", "user-custom-provider"
	mock.selected = copilotSelection{}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock.mu.Lock()
	mock.selected = created
	mock.mu.Unlock()
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.selected != created {
		t.Fatal("a new session's explicit custom provider was replaced")
	}
}

func TestCopilotAutomaticConnectionRetriesRegistrationWithoutLosingEnablement(t *testing.T) {
	a := testApp(t, nil)
	a.state = session{Token: "account-b"}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock := mockCopilotSetup(t, a)
	mock.selected.ProviderID = ""
	mock.fail = "upsert_model_provider"
	if err := a.syncCopilot(); err == nil || !strings.Contains(err.Error(), "synthetic app rejection") || !a.snapshot().Copilot {
		t.Fatal("background registration failure was hidden or disabled the bridge", err)
	}
	mock.mu.Lock()
	mock.fail = ""
	mock.mu.Unlock()
	if err := a.syncCopilot(); err != nil {
		t.Fatal("background registration did not recover", err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.selected.ProviderID != copilotProviderID {
		t.Fatal("registration recovery still required a manual toggle")
	}
}

func TestCopilotAutomaticConnectionRechecksIdleState(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	created := mock.selected
	created.ID, created.ProviderID = "new-session", ""
	mock.selected = copilotSelection{}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock.mu.Lock()
	mock.sessions[created.ID] = created
	mock.onRequest = func(command object) {
		if command["type"] == "list_model_providers" {
			running := mock.sessions[created.ID]
			running.Running = 1
			mock.sessions[created.ID] = running
		}
	}
	mock.mu.Unlock()
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	if mock.sessions[created.ID].ProviderID == copilotProviderID {
		t.Error("a session that started running during setup was switched")
	}
	mock.onRequest = nil
	mock.sessions[created.ID] = created
	mock.mu.Unlock()
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.sessions[created.ID].ProviderID != copilotProviderID {
		t.Fatal("a deferred session was not automatically retried when idle")
	}
}

func TestCopilotAutomaticConnectionDoesNotRetryUncertainSwap(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	created := mock.selected
	created.ID, created.ProviderID = "new-session", ""
	mock.selected = copilotSelection{}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock.mu.Lock()
	mock.sessions[created.ID] = created
	mock.drop = "set_session_model"
	mock.mu.Unlock()
	if err := a.syncCopilot(); err == nil || !a.snapshot().Copilot {
		t.Fatal("uncertain automatic swap was hidden or disabled its endpoint", err)
	}
	mock.mu.Lock()
	mock.drop = ""
	mock.requests = nil
	mock.mu.Unlock()
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	for _, command := range mock.requests {
		if command["type"] == "set_session_model" {
			t.Fatal("an uncertain swap was replayed automatically")
		}
	}
}

func TestCopilotAutomaticRegistrationReconnectsWithoutReswitchingSessions(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	path := filepath.Join(a.options.Home, ".copilot", "run", "ws.port")
	port, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := a.syncCopilot(); err != nil || !a.snapshot().Copilot {
		t.Fatal("temporary app shutdown lost enablement", err)
	}
	if err := writeAtomic(path, port); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	selected := mock.selected
	mock.requests = nil
	mock.mu.Unlock()
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	registered := false
	for _, command := range mock.requests {
		if command["type"] == "list_model_providers" {
			registered = true
		}
		if command["type"] == "set_session_model" {
			t.Fatal("reconnecting replayed a session switch")
		}
	}
	if !registered || mock.selected != selected {
		t.Fatal("reconnecting did not verify registration or changed a session")
	}
}

func TestCopilotAutomaticConnectionPreservesManualProviderAfterReload(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock.mu.Lock()
	mock.selected.ProviderID = ""
	changed := mock.selected
	mock.requests = nil
	mock.mu.Unlock()
	a.copilotRouting = copilotRouting{}
	if err := a.syncCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.selected != changed {
		t.Fatal("resuming an enabled bridge overrode a previous manual provider change")
	}
	for _, command := range mock.requests {
		if command["type"] == "set_session_model" {
			t.Fatal("resuming an enabled bridge attempted to steal back a session")
		}
	}
}

func TestCopilotBackgroundWatcherConnectsAndStops(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	mock.selected.ProviderID = ""
	mock.selected.Running = 1
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	a.startCopilotWatcher()
	a.startCopilotWatcher()
	mock.mu.Lock()
	mock.selected.Running = 0
	mock.mu.Unlock()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("background watcher never connected the idle session")
		case <-ticker.C:
			mock.mu.Lock()
			connected := mock.selected.ProviderID == copilotProviderID
			mock.mu.Unlock()
			if connected {
				if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 || a.snapshot().Copilot {
					t.Fatal("watcher prevented a safe disable", result.Body.String())
				}
				if err := a.Stop(); err != nil {
					t.Fatal("watcher did not stop cleanly", err)
				}
				return
			}
		}
	}
}
