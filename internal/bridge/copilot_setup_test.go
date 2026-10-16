package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type testCopilotControl struct {
	mu              sync.Mutex
	selected        copilotSelection
	sessions        map[string]copilotSelection
	providers       []object
	models          []object
	requests        []object
	fail            string
	drop            string
	dropAfter       string
	dropAfterResult bool
	swap            bool
	swapError       string
	runtimeRunning  map[string]bool
	livenessStatus  string
	onRequest       func(object)
	onResult        func(object)
	onLiveness      func(object)
}

func mockCopilotSetup(t *testing.T, a *App) *testCopilotControl {
	t.Helper()
	mock := &testCopilotControl{
		selected:       copilotSelection{ID: "selected-session", Model: "gpt-test", ProviderID: "original-provider", Location: "local", Host: "local", ReasoningEffort: "high", ContextTier: "long_context"},
		sessions:       map[string]copilotSelection{},
		providers:      []object{{"id": "original-provider", "name": "Existing", "kind": "custom", "settings": object{"baseUrl": "https://existing.example/v1"}}},
		models:         []object{},
		runtimeRunning: map[string]bool{},
		livenessStatus: "responsive",
	}
	a.copilotSession = func() (copilotSelection, error) {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		return mock.selected, nil
	}
	a.copilotSessions = func() ([]copilotSelection, error) {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		sessions := []copilotSelection{}
		if mock.selected.ProviderID == copilotProviderID {
			sessions = append(sessions, mock.selected)
		}
		for _, selected := range mock.sessions {
			if selected.ProviderID == copilotProviderID {
				sessions = append(sessions, selected)
			}
		}
		return sessions, nil
	}
	a.copilotLocalSessions = func() ([]copilotSelection, error) {
		mock.mu.Lock()
		defer mock.mu.Unlock()
		sessions := []copilotSelection{}
		if mock.selected.ID != "" && mock.selected.Location == "local" && mock.selected.Host == "local" && mock.selected.Remote == "" {
			sessions = append(sessions, mock.selected)
		}
		for _, selected := range mock.sessions {
			if selected.ID != mock.selected.ID && selected.Location == "local" && selected.Host == "local" && selected.Remote == "" {
				sessions = append(sessions, selected)
			}
		}
		return sessions, nil
	}
	a.rawModels = []object{{"id": "gpt-test", "name": "Test model", "supported_endpoints": []any{"/responses"},
		"capabilities": object{"type": "chat", "limits": object{"max_prompt_tokens": 100000, "max_output_tokens": 10000},
			"supports": object{"reasoning_effort": []any{"low", "high"}}}}}
	a.rawAt = time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "app-control-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		for {
			var command object
			if err := wsjson.Read(r.Context(), conn, &command); err != nil {
				return
			}
			mock.mu.Lock()
			mock.requests = append(mock.requests, command)
			if mock.drop == command["type"] {
				mock.mu.Unlock()
				return
			}
			if mock.onRequest != nil {
				mock.onRequest(command)
			}
			reply := object{"request_id": command["request_id"]}
			if mock.fail == command["type"] {
				reply["type"], reply["message"] = "error", "synthetic app rejection"
			} else {
				switch command["type"] {
				case "list_model_providers":
					reply["type"], reply["providers"] = "model_providers_list", mock.providers
				case "list_provider_models":
					reply["type"], reply["models"] = "provider_models_list", mock.models
				case "upsert_model_provider":
					provider := obj(command["provider"])
					provider["hasSecret"] = true
					secret := obj(command["secret"])
					if provider["id"] != copilotProviderID || provider["kind"] != "custom" ||
						nested(provider, "settings", "baseUrl") != a.CopilotURL() ||
						nested(provider, "settings", "managedBy") != "Kobashi" ||
						secret["kind"] != "apiKey" || secret["value"] != "PROXY_MANAGED" {
						t.Error("invalid or credential-leaking provider registration", provider["id"])
					}
					if len(mock.providers) == 1 {
						mock.providers = append(mock.providers, provider)
					} else {
						mock.providers[1] = provider
					}
					reply["type"], reply["provider"] = "model_provider_upserted", provider
				case "upsert_provider_model":
					model := obj(command["model"])
					found := false
					for i, existing := range mock.models {
						if existing["providerId"] == model["providerId"] && existing["modelId"] == model["modelId"] {
							if model["id"] != existing["id"] {
								t.Error("re-enabling attempted a duplicate model insert")
							}
							mock.models[i] = model
							found = true
							break
						}
					}
					if !found {
						model["id"] = fmt.Sprintf("registered-model-%d", len(mock.models))
						mock.models = append(mock.models, model)
					}
					reply["type"], reply["model"] = "provider_model_upserted", model
				case "set_session_model":
					selected := mock.selected
					if command["session_id"] != selected.ID {
						selected = mock.sessions[str(command["session_id"])]
					}
					if selected.ID == "" || selected.Running != 0 || !a.snapshot().Copilot {
						t.Error("switched a different/running session or selected an inactive bridge")
					}
					if command["model"] == selected.Model && command["provider_id"] == copilotProviderID &&
						command["reasoning_effort"] != selected.ReasoningEffort {
						t.Error("switching provider lost a supported reasoning effort")
					}
					if mock.swap {
						if err := wsjson.Write(r.Context(), conn, object{"type": "session_provider_swap_started", "session_id": command["session_id"]}); err != nil {
							mock.mu.Unlock()
							return
						}
					}
					if mock.swapError == "" {
						selected.Model, selected.ProviderID = str(command["model"]), str(command["provider_id"])
						selected.ReasoningEffort, selected.ContextTier = str(command["reasoning_effort"]), str(command["context_tier"])
						if command["session_id"] == mock.selected.ID {
							mock.selected = selected
						} else {
							mock.sessions[selected.ID] = selected
						}
					}
					reply["type"], reply["session_id"] = "session_model_result", command["session_id"]
					reply["model"], reply["provider_id"] = command["model"], command["provider_id"]
					if mock.onResult != nil && command["provider_id"] == copilotProviderID {
						mock.onResult(reply)
					}
				case "probe_session_liveness":
					if command["recover"] != false {
						t.Error("checking activity attempted to recover or restart a session")
					}
					selected := mock.selected
					if command["session_id"] != selected.ID {
						selected = mock.sessions[str(command["session_id"])]
					}
					reply["type"], reply["session_id"] = "session_liveness_result", command["session_id"]
					reply["status"] = mock.livenessStatus
					reply["is_running"] = selected.Running != 0 || mock.runtimeRunning[selected.ID]
					if mock.onLiveness != nil {
						mock.onLiveness(reply)
					}
				default:
					t.Error("unexpected app command", command["type"])
					reply["type"], reply["message"] = "error", "unexpected command"
				}
			}
			if mock.dropAfter == command["type"] {
				mock.mu.Unlock()
				return
			}
			// Broadcasts from unrelated requests must not satisfy this request.
			if reply["type"] == "model_providers_list" || reply["type"] == "provider_models_list" || reply["type"] == "session_liveness_result" {
				if err := wsjson.Write(r.Context(), conn, object{"type": reply["type"], "providers": []object{}, "models": []object{}}); err != nil {
					mock.mu.Unlock()
					return
				}
				if reply["type"] == "session_liveness_result" {
					if err := wsjson.Write(r.Context(), conn, object{"type": reply["type"], "session_id": command["session_id"],
						"status": "responsive", "is_running": false}); err != nil {
						mock.mu.Unlock()
						return
					}
				}
			}
			err = wsjson.Write(r.Context(), conn, object{"type": reply["type"], "request_id": "unrelated-request"})
			if err == nil {
				err = wsjson.Write(r.Context(), conn, reply)
			}
			if mock.dropAfterResult && command["type"] == "set_session_model" {
				mock.mu.Unlock()
				return
			}
			if err == nil && command["type"] == "set_session_model" && mock.swap {
				err = wsjson.Write(r.Context(), conn, object{"type": "session_provider_swap_completed", "session_id": command["session_id"],
					"new_model": command["model"], "error": mock.swapError})
			}
			mock.mu.Unlock()
			if err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"ws.port": port, "ws.token": "app-control-token"} {
		if err := writeAtomic(filepath.Join(a.options.Home, ".copilot", "run", name), []byte(fmt.Sprintf("%s\n%d\n", value, os.Getpid()))); err != nil {
			t.Fatal(err)
		}
	}
	return mock
}

func TestCopilotAutomaticSetupSelectsCurrentSession(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	for range 2 {
		if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 || !a.snapshot().Copilot ||
			!strings.Contains(result.Body.String(), "registered and selected") {
			t.Fatal("automatic setup failed", result.Body.String())
		}
		mock.mu.Lock()
		if mock.selected.ProviderID != copilotProviderID || mock.selected.Model != "gpt-test" ||
			len(mock.providers) != 2 || mock.providers[0]["id"] != "original-provider" || len(mock.models) != 1 ||
			mock.models[0]["wireApiOverride"] != "responses" || number(mock.models[0]["maxPromptTokens"]) != 100000 {
			t.Error("setup changed another provider or lost model metadata")
		}
		mock.mu.Unlock()
		if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 || a.snapshot().Copilot {
			t.Fatal("disabling did not stop the bridge", result.Body.String())
		}
	}
}

func TestCopilotRegistrationRepairsOnlyChangedConfiguration(t *testing.T) {
	for _, field := range []string{"credential", "model"} {
		t.Run(field, func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			mock.selected = copilotSelection{}
			if _, err := a.enableCopilot(); err != nil {
				t.Fatal(err)
			}
			mock.mu.Lock()
			if field == "credential" {
				mock.providers[1]["hasSecret"] = false
			} else {
				mock.models[0]["displayName"] = "Stale model metadata"
			}
			mock.requests = nil
			mock.mu.Unlock()
			if _, err := a.enableCopilot(); err != nil {
				t.Fatal(err)
			}
			mock.mu.Lock()
			defer mock.mu.Unlock()
			providers, models := 0, 0
			for _, command := range mock.requests {
				if command["type"] == "upsert_model_provider" {
					providers++
				}
				if command["type"] == "upsert_provider_model" {
					models++
				}
			}
			if (field == "credential" && (providers != 1 || models != 0)) ||
				(field == "model" && (providers != 0 || models != 1)) {
				t.Fatal("registration did not repair only the changed configuration", providers, models)
			}
		})
	}
}

func TestCopilotDisableRestoresOriginalSelection(t *testing.T) {
	for _, operation := range []string{"/api/toggle-copilot", "/api/disconnect"} {
		t.Run(operation, func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			original := mock.selected
			if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
				t.Fatal(result.Body.String())
			}
			if _, err := a.enableCopilot(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(a.path("copilot-restore.json"))
			var saved map[string]copilotSelection
			if err != nil || json.Unmarshal(data, &saved) != nil || saved[original.ID] != original {
				t.Fatal("original selection was not persisted before the switch", err)
			}
			mock.mu.Lock()
			mock.swap = true
			mock.mu.Unlock()
			result := controlCall(a, operation, object{})
			if result.Code != 200 || a.snapshot().Copilot {
				t.Fatal("bridge was not disabled after restoring its session", result.Body.String())
			}
			mock.mu.Lock()
			defer mock.mu.Unlock()
			if mock.selected != original || len(mock.providers) != 2 || len(mock.models) != 1 {
				t.Fatal("original provider/model/effort/context was not restored, or provider registration was deleted")
			}
			if operation == "/api/toggle-copilot" {
				data, err = os.ReadFile(a.path("session.json"))
				var saved session
				if err != nil || json.Unmarshal(data, &saved) != nil || saved.Copilot {
					t.Fatal("disabled state was not persisted", err)
				}
			}
		})
	}
}

func TestCopilotDisableRestoresSessionsAfterReloadWithoutChangingOtherSessions(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	first := mock.selected
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	second := first
	second.ID, second.Model, second.ProviderID = "second-session", "another-model", "another-provider"
	mock.mu.Lock()
	mock.sessions[first.ID] = mock.selected
	mock.selected = second
	mock.mu.Unlock()
	// Register another session while the endpoint is already enabled.
	if _, err := a.enableCopilot(); err != nil {
		t.Fatal(err)
	}
	unrelated := first
	unrelated.ID, unrelated.Running = "unrelated-session", 1
	mock.mu.Lock()
	mock.sessions[unrelated.ID] = unrelated
	mock.sessions[second.ID] = mock.selected
	mock.selected = copilotSelection{}
	mock.mu.Unlock()
	reloaded, err := New(a.options)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.initialize()
	t.Cleanup(func() { reloaded.Stop() })
	reloaded.copilotSessions = a.copilotSessions
	reloaded.copilotSession = a.copilotSession
	reloaded.rawModels, reloaded.rawAt = a.rawModels, a.rawAt
	if !reloaded.snapshot().Copilot {
		t.Fatal("reload lost the enabled Copilot preference")
	}
	result := controlCall(reloaded, "/api/toggle-copilot", object{})
	if result.Code != 200 || reloaded.snapshot().Copilot {
		t.Fatal("reload lost the original selections", result.Body.String())
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.sessions[first.ID] != first || mock.sessions[second.ID] != second || mock.sessions[unrelated.ID] != unrelated {
		t.Fatal("restoration lost an original selection or changed an unrelated session")
	}
}

func TestCopilotDisableRestoresDefaultGitHubProvider(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	mock.selected.ProviderID = ""
	original := mock.selected
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock.mu.Lock()
	mock.onRequest = func(command object) {
		if command["type"] == "set_session_model" && command["provider_id"] != nil {
			t.Error("restoring the app's default provider must send JSON null, not an empty provider identifier")
		}
	}
	mock.mu.Unlock()
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 || a.snapshot().Copilot {
		t.Fatal("default GitHub provider was not restored", result.Body.String())
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.selected != original {
		t.Fatal("restoring the default provider changed its model or settings")
	}
}

func TestCopilotDisablePreservesUserProviderChanges(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock.mu.Lock()
	mock.selected.ProviderID, mock.selected.Model = "user-chosen-provider", "user-chosen-model"
	changed := mock.selected
	mock.requests = nil
	mock.mu.Unlock()
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 || a.snapshot().Copilot {
		t.Fatal("a session that no longer uses Kobashi prevented disabling", result.Body.String())
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.selected != changed || len(mock.requests) != 0 {
		t.Fatal("disabling overwrote a user-selected provider")
	}
}

func TestCopilotDisableRetriesAfterPartialRestoration(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	first := mock.selected
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	second := first
	second.ID = "second-session"
	mock.mu.Lock()
	mock.sessions[first.ID], mock.selected = mock.selected, second
	mock.mu.Unlock()
	if _, err := a.enableCopilot(); err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	switches := 0
	mock.onRequest = func(command object) {
		if command["type"] == "set_session_model" {
			switches++
			if switches == 2 {
				mock.fail = "set_session_model"
			}
		}
	}
	mock.mu.Unlock()
	result := controlCall(a, "/api/toggle-copilot", object{})
	if result.Code != 400 || !a.snapshot().Copilot {
		t.Fatal("partial restoration disabled the bridge", result.Body.String())
	}
	mock.mu.Lock()
	mock.fail, mock.onRequest = "", nil
	mock.mu.Unlock()
	result = controlCall(a, "/api/toggle-copilot", object{})
	if result.Code != 200 || a.snapshot().Copilot {
		t.Fatal("partial restoration could not be retried", result.Body.String())
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.sessions[first.ID] != first || mock.selected != second {
		t.Fatal("retry did not preserve both original selections")
	}
}

func TestCopilotDisableKeepsEnabledStateWhenSaveFails(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	original := mock.selected
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	if err := os.Remove(a.path("session.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(a.path("session.json"), 0700); err != nil {
		t.Fatal(err)
	}
	result := controlCall(a, "/api/toggle-copilot", object{})
	if result.Code != 400 || !a.snapshot().Copilot {
		t.Fatal("failed persistence left an unsaved disabled state", result.Body.String())
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.selected != original {
		t.Fatal("failed bridge persistence undid the restored native provider")
	}
}

func TestCopilotDisconnectDoesNotInterruptRunningSession(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	mock.mu.Lock()
	mock.selected.Running = 1
	mock.mu.Unlock()
	result := controlCall(a, "/api/disconnect", object{})
	if result.Code != 400 || !a.snapshot().Copilot || a.snapshot().Token != "account-b" {
		t.Fatal("disconnect cut off a running bridge session", result.Body.String())
	}
}

func TestCopilotDisableKeepsEndpointOnWhenRestorationIsUnsafe(t *testing.T) {
	for _, failure := range []string{"running", "remote", "missing-backup", "invalid-backup", "unavailable-database",
		"unavailable-control", "rejected", "lost-acknowledgement", "swap-failed", "changed-during-restore", "new-bridge-session"} {
		t.Run(failure, func(t *testing.T) {
			a := testApp(t, func(*http.Request) (*http.Response, error) {
				return response(200, object{"id": "still-working"}), nil
			})
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			a.bearer, a.expiry = "cached-token", float64(time.Now().Add(time.Hour).Unix())
			if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
				t.Fatal(result.Body.String())
			}
			mock.mu.Lock()
			switch failure {
			case "running":
				mock.selected.Running = 1
			case "remote":
				mock.selected.Host = "remote-host"
			case "missing-backup":
				if err := os.Remove(a.path("copilot-restore.json")); err != nil {
					t.Fatal(err)
				}
			case "invalid-backup":
				if err := writeAtomic(a.path("copilot-restore.json"), []byte(`null`)); err != nil {
					t.Fatal(err)
				}
			case "unavailable-database":
				a.copilotSessions = func() ([]copilotSelection, error) {
					return nil, errors.New("selection database unavailable")
				}
			case "unavailable-control":
				if err := os.Remove(filepath.Join(a.options.Home, ".copilot", "run", "ws.port")); err != nil {
					t.Fatal(err)
				}
			case "rejected":
				mock.fail = "set_session_model"
			case "lost-acknowledgement":
				mock.drop = "set_session_model"
			case "swap-failed":
				mock.swap, mock.swapError = true, "runtime startup failed"
			case "changed-during-restore":
				reads := 0
				read := a.copilotSessions
				a.copilotSessions = func() ([]copilotSelection, error) {
					reads++
					if reads == 2 {
						mock.mu.Lock()
						mock.selected.Running = 1
						mock.mu.Unlock()
					}
					return read()
				}
			case "new-bridge-session":
				mock.onRequest = func(command object) {
					if command["type"] == "set_session_model" {
						extra := mock.selected
						extra.ID = "new-bridge-session"
						mock.sessions[extra.ID] = extra
					}
				}
			}
			mock.requests = nil
			mock.mu.Unlock()
			result := controlCall(a, "/api/toggle-copilot", object{})
			if result.Code != 400 || !a.snapshot().Copilot {
				t.Fatal("unsafe restoration disabled its inference endpoint", result.Body.String())
			}
			var saved session
			data, err := os.ReadFile(a.path("session.json"))
			if err != nil || json.Unmarshal(data, &saved) != nil || !saved.Copilot {
				t.Fatal("unsafe disable changed the persisted bridge state", err)
			}
			w := httptest.NewRecorder()
			a.copilotApp(w, copilotRequest("POST", "/v1/responses", `{"model":"gpt-test"}`))
			if w.Code != 200 {
				t.Fatal("failed disable broke subsequent inference", w.Code, w.Body.String())
			}
			mock.mu.Lock()
			defer mock.mu.Unlock()
			if failure != "rejected" && failure != "lost-acknowledgement" && failure != "swap-failed" && failure != "new-bridge-session" {
				for _, command := range mock.requests {
					if command["type"] == "set_session_model" {
						t.Fatal("unsafe disable attempted a session switch")
					}
				}
			}
		})
	}
}

func TestCopilotSetupDoesNotSwitchWithoutSavingOriginalSelection(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	if err := os.MkdirAll(a.path("copilot-restore.json"), 0700); err != nil {
		t.Fatal(err)
	}
	result := controlCall(a, "/api/toggle-copilot", object{})
	if result.Code != 400 || !a.snapshot().Copilot {
		t.Fatal("could not persist the original selection but switched anyway", result.Body.String())
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.selected.ProviderID != "original-provider" {
		t.Fatal("original provider was changed without a durable backup")
	}
}

func TestCopilotAutomaticSetupAcceptsQualifiedModelIDs(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	a.rawModels = append([]object{{"id": "first-model", "supported_endpoints": []any{"/responses"}}}, a.rawModels...)
	mock.selected.Model = mock.selected.ProviderID + "/gpt-test"
	mock.onResult = func(reply object) {
		if reply["model"] != "gpt-test" {
			t.Error("qualified selection did not preserve the current wire model", reply["model"])
		}
		mock.selected.Model = copilotProviderID + "/gpt-test"
		reply["model"] = mock.selected.Model
	}
	mock.onRequest = func(command object) {
		if command["type"] == "set_session_model" && command["reasoning_effort"] != "high" {
			t.Error("qualified selection lost its supported reasoning effort")
		}
	}
	for range 2 {
		result := controlCall(a, "/api/toggle-copilot", object{})
		if result.Code != 200 || !a.snapshot().Copilot || !strings.Contains(result.Body.String(), "registered and selected") {
			t.Fatal("qualified model acknowledgement was rejected", result.Body.String())
		}
		if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 || a.snapshot().Copilot {
			t.Fatal("disabling the bridge failed", result.Body.String())
		}
	}
}

func TestCopilotAutomaticSetupRejectsMismatchedModelResult(t *testing.T) {
	for _, name := range []string{"provider", "model", "qualified-provider", "missing-provider"} {
		t.Run(name, func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			mock.onResult = func(reply object) {
				switch name {
				case "provider":
					reply["provider_id"] = "another-provider"
					reply["model"] = copilotProviderID + "/gpt-test"
				case "model":
					reply["model"] = copilotProviderID + "/another-model"
				case "qualified-provider":
					reply["model"] = "another-provider/gpt-test"
				case "missing-provider":
					delete(reply, "provider_id")
				}
			}
			result := controlCall(a, "/api/toggle-copilot", object{})
			if result.Code != 400 || !a.snapshot().Copilot || !strings.Contains(result.Body.String(), "did not confirm") {
				t.Fatal("unconfirmed model switch was treated as success or disabled the registered bridge", result.Body.String())
			}
		})
	}
}

func TestCopilotAutomaticSetupRegistersWithoutSwitchingUnsafeSelections(t *testing.T) {
	for _, name := range []string{"running", "cloud", "remote", "no-session", "no-model", "selection-unavailable", "started-during-setup", "tab-changed", "provider-conflict"} {
		t.Run(name, func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			switch name {
			case "running":
				mock.selected.Running = 1
			case "cloud":
				mock.selected.Location = "cloud"
			case "remote":
				mock.selected.Host = "remote-host"
			case "no-session":
				mock.selected.ID = ""
			case "no-model":
				mock.selected.Model = ""
			case "selection-unavailable":
				a.copilotSession = func() (copilotSelection, error) {
					return copilotSelection{}, errors.New("selection database is unavailable")
				}
			case "started-during-setup", "tab-changed":
				mock.onRequest = func(command object) {
					if command["type"] == "upsert_provider_model" {
						if name == "tab-changed" {
							mock.selected.ID = "another-session"
						} else {
							mock.selected.Running = 1
						}
					}
				}
			case "provider-conflict":
				mock.providers = append(mock.providers, object{"id": copilotProviderID, "kind": "custom", "name": "User-owned"})
			}
			result := controlCall(a, "/api/toggle-copilot", object{})
			if name == "provider-conflict" {
				if result.Code != 400 || a.snapshot().Copilot {
					t.Fatal("conflicting provider was overwritten", result.Body.String())
				}
			} else if result.Code != 200 || !a.snapshot().Copilot || !strings.Contains(result.Body.String(), "no session was switched") {
				t.Fatal("provider registration was blocked by session selection", result.Body.String())
			}
			mock.mu.Lock()
			defer mock.mu.Unlock()
			for _, command := range mock.requests {
				if command["type"] == "set_session_model" {
					t.Fatal("unsafe setup attempted to switch a session")
				}
			}
		})
	}
}

func TestCopilotAutomaticSetupSurfacesAppErrors(t *testing.T) {
	for _, failure := range []string{"upsert_model_provider", "upsert_provider_model", "set_session_model"} {
		t.Run(failure, func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			mock.fail = failure
			result := controlCall(a, "/api/toggle-copilot", object{})
			if result.Code != 400 || a.snapshot().Copilot != (failure == "set_session_model") || !strings.Contains(result.Body.String(), "synthetic app rejection") {
				t.Fatal("app error was hidden or enabled state was not rolled back", result.Body.String())
			}
		})
	}
}

func TestCopilotAutomaticSetupWaitsForProviderSwap(t *testing.T) {
	for _, failure := range []string{"", "synthetic runtime startup failed"} {
		t.Run(failure, func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			mock.swap, mock.swapError = true, failure
			result := controlCall(a, "/api/toggle-copilot", object{})
			if failure == "" {
				if result.Code != 200 || !a.snapshot().Copilot {
					t.Fatal("successful runtime swap was rejected", result.Body.String())
				}
			} else if result.Code != 400 || !a.snapshot().Copilot || !strings.Contains(result.Body.String(), failure) {
				t.Fatal("runtime startup failure was reported as success", result.Body.String())
			}
		})
	}
}

func TestCopilotAutomaticSetupVerifiesPersistedRegistration(t *testing.T) {
	for _, field := range []string{"credential", "model"} {
		t.Run(field, func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			mock.onRequest = func(command object) {
				if command["type"] == "list_model_providers" && len(mock.models) != 0 && field == "credential" {
					mock.providers[1]["hasSecret"] = false
				}
				if command["type"] == "list_provider_models" && len(mock.models) != 0 && field == "model" {
					mock.models = []object{}
				}
			}
			result := controlCall(a, "/api/toggle-copilot", object{})
			if result.Code != 400 || a.snapshot().Copilot || !strings.Contains(result.Body.String(), "did not persist") {
				t.Fatal("optimistic acknowledgement was treated as verified registration", result.Body.String())
			}
		})
	}
}

func TestCopilotAutomaticSetupKeepsEndpointForUnknownSwitchOutcome(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	mock.drop = "set_session_model"
	result := controlCall(a, "/api/toggle-copilot", object{})
	if result.Code != 400 || !a.snapshot().Copilot || !strings.Contains(result.Body.String(), "Bridge stays enabled") {
		t.Fatal("an unconfirmed switch disabled its potential inference endpoint", result.Body.String())
	}
	var saved session
	data, err := os.ReadFile(a.path("session.json"))
	if err != nil || json.Unmarshal(data, &saved) != nil || !saved.Copilot {
		t.Fatal("unknown-outcome state was not persisted", err)
	}
}

func TestCopilotDisconnectedSwitchIsNeverReplayedOrReportedAsConfirmed(t *testing.T) {
	for _, phase := range []string{"before-command", "after-persisting-selection", "before-runtime-completion"} {
		t.Run(phase, func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			switch phase {
			case "before-command":
				mock.drop = "set_session_model"
			case "after-persisting-selection":
				mock.dropAfter = "set_session_model"
			case "before-runtime-completion":
				mock.swap, mock.dropAfterResult = true, true
			}
			result := controlCall(a, "/api/toggle-copilot", object{})
			if result.Code != 400 || !a.snapshot().Copilot ||
				!strings.Contains(result.Body.String(), "Copilot disconnected during the model switch") ||
				!strings.Contains(result.Body.String(), "select Kobashi") ||
				strings.Contains(result.Body.String(), "failed to read JSON") {
				t.Fatal("uncertain switch was hidden, disabled inference, or leaked raw transport details", result.Body.String())
			}
			saved, err := a.copilotRestoreSelections()
			if err != nil || len(saved) != 1 || saved["selected-session"].ProviderID != "original-provider" {
				t.Fatal("disconnect lost the original provider needed for restoration", saved, err)
			}
			mock.mu.Lock()
			mock.drop, mock.dropAfter, mock.dropAfterResult = "", "", false
			mock.requests = nil
			mock.mu.Unlock()
			if err := a.syncCopilot(); err != nil {
				t.Fatal("watcher could not inspect after a lost confirmation", err)
			}
			mock.mu.Lock()
			defer mock.mu.Unlock()
			for _, command := range mock.requests {
				if command["type"] == "set_session_model" {
					t.Fatal("watcher replayed a switch whose outcome is unknown")
				}
			}
		})
	}
}

func TestCopilotChecksRuntimeActivityBeforeSwitchingOrRestoring(t *testing.T) {
	for _, restoring := range []bool{false, true} {
		t.Run(fmt.Sprintf("restore-%t", restoring), func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			if restoring {
				if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
					t.Fatal(result.Body.String())
				}
			}
			mock.mu.Lock()
			original := mock.selected
			mock.runtimeRunning[original.ID] = true
			mock.requests = nil
			mock.mu.Unlock()
			result := controlCall(a, "/api/toggle-copilot", object{})
			if !a.snapshot().Copilot || (restoring && result.Code != 400) || (!restoring && result.Code != 200) {
				t.Fatal("live runtime activity did not defer the switch while preserving inference", result.Body.String())
			}
			mock.mu.Lock()
			for _, command := range mock.requests {
				if command["type"] == "set_session_model" {
					t.Error("stale idle database state interrupted a running runtime")
				}
			}
			if mock.selected != original {
				t.Error("checking activity changed a session")
			}
			mock.runtimeRunning[original.ID] = false
			mock.mu.Unlock()
			if restoring {
				if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 || a.snapshot().Copilot {
					t.Fatal("safe restore failed after the runtime became idle", result.Body.String())
				}
			} else if err := a.syncCopilot(); err != nil {
				t.Fatal("automatic connection failed when runtime became idle", err)
			} else {
				mock.mu.Lock()
				if mock.selected.ProviderID != copilotProviderID {
					t.Error("deferred runtime was not connected after it became idle")
				}
				mock.mu.Unlock()
			}
		})
	}
}

func TestCopilotWaitsForResponsiveRuntimeAndRejectsInvalidActivity(t *testing.T) {
	for _, failure := range []string{"not-responsive", "missing-activity", "wrong-session", "rejected", "disconnected", "provisional"} {
		t.Run(failure, func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			pending := failure == "not-responsive" || failure == "provisional"
			switch failure {
			case "not-responsive":
				mock.livenessStatus = "unknown"
			case "missing-activity":
				mock.onLiveness = func(reply object) { delete(reply, "is_running") }
			case "wrong-session":
				mock.onLiveness = func(reply object) { reply["session_id"] = "another-session" }
			case "rejected":
				mock.fail = "probe_session_liveness"
			case "disconnected":
				mock.drop = "probe_session_liveness"
			case "provisional":
				mock.selected.Provisional = 1
			}
			result := controlCall(a, "/api/toggle-copilot", object{})
			if !a.snapshot().Copilot || (pending && result.Code != 200) || (!pending && result.Code != 400) {
				t.Fatal("unknown/unsafe runtime state was not handled explicitly", result.Body.String())
			}
			mock.mu.Lock()
			defer mock.mu.Unlock()
			for _, command := range mock.requests {
				if command["type"] == "set_session_model" {
					t.Fatal("unsafe or unverified runtime was switched")
				}
			}
		})
	}
}

func TestCopilotRechecksSelectionAfterRuntimeProbe(t *testing.T) {
	for _, restoring := range []bool{false, true} {
		t.Run(fmt.Sprintf("restore-%t", restoring), func(t *testing.T) {
			a := testApp(t, nil)
			mock := mockCopilotSetup(t, a)
			a.state = session{Token: "account-b"}
			if restoring {
				if result := controlCall(a, "/api/toggle-copilot", object{}); result.Code != 200 {
					t.Fatal(result.Body.String())
				}
			}
			mock.mu.Lock()
			mock.requests = nil
			mock.onLiveness = func(object) { mock.selected.Running = 1 }
			mock.mu.Unlock()
			result := controlCall(a, "/api/toggle-copilot", object{})
			if !a.snapshot().Copilot || (restoring && result.Code != 400) || (!restoring && result.Code != 200) {
				t.Fatal("a session changed during the runtime check without safe deferral", result.Body.String())
			}
			mock.mu.Lock()
			for _, command := range mock.requests {
				if command["type"] == "set_session_model" {
					t.Error("a stale runtime probe allowed switching a newly running session")
				}
			}
			mock.onLiveness = nil
			mock.selected.Running = 0
			mock.mu.Unlock()
		})
	}
}

func TestCopilotAutomaticSetupKeepsSavedStateWhenRollbackFails(t *testing.T) {
	a := testApp(t, nil)
	mock := mockCopilotSetup(t, a)
	a.state = session{Token: "account-b"}
	mock.fail = "upsert_model_provider"
	mock.onRequest = func(command object) {
		if command["type"] == mock.fail {
			if err := os.Remove(a.path("session.json")); err != nil {
				t.Error(err)
			}
			if err := os.Mkdir(a.path("session.json"), 0700); err != nil {
				t.Error(err)
			}
		}
	}
	result := controlCall(a, "/api/toggle-copilot", object{})
	if result.Code != 400 || !a.snapshot().Copilot || !strings.Contains(result.Body.String(), "could not save its disabled state") {
		t.Fatal("failed rollback silently contradicted the last saved enabled state", result.Body.String())
	}
}

func TestCopilotControlRejectsMismatchedOwnerAndRedactsToken(t *testing.T) {
	a := testApp(t, nil)
	mockCopilotSetup(t, a)
	if err := writeAtomic(filepath.Join(a.options.Home, ".copilot", "run", "ws.token"), []byte("private-control-token\n1\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := connectCopilotControl(context.Background(), a.options.Home); err == nil || strings.Contains(err.Error(), "private-control-token") {
		t.Fatal("mismatched control owners accepted or token leaked", err)
	}
	if err := writeAtomic(filepath.Join(a.options.Home, ".copilot", "run", "ws.token"), []byte(fmt.Sprintf("private-control-token\n%d\n", os.Getpid()))); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := connectCopilotControl(ctx, a.options.Home); err == nil || strings.Contains(err.Error(), "private-control-token") {
		t.Fatal("invalid control authentication accepted or token leaked", err)
	}
	err := errors.New("ws://127.0.0.1/?token=secret%2Btoken secret+token")
	if got := redactCopilotControlError(err, "secret+token"); strings.Contains(got, "secret") {
		t.Fatal("control token leaked", got)
	}
}

func TestCopilotSetupModelPrefersCurrentAndUsesAdvertisedProtocol(t *testing.T) {
	models := []object{
		{"id": "disabled", "supported_endpoints": []any{"/responses"}, "policy": object{"state": "disabled"}},
		{"id": "first", "supported_endpoints": []any{"/responses"}},
		{"id": "current", "supported_endpoints": []any{"/chat/completions"}},
	}
	model, wire, err := copilotSetupModel(models, "current")
	if err != nil || model["id"] != "current" || wire != "completions" {
		t.Fatal(model, wire, err)
	}
	model, wire, err = copilotSetupModel(models, "unavailable")
	if err != nil || model["id"] != "first" || wire != "responses" {
		t.Fatal(model, wire, err)
	}
	if _, _, err := copilotSetupModel(models[:1], "disabled"); err == nil {
		t.Fatal("disabled-only catalog was accepted")
	}
}

func TestInstalledCopilotProviderRegistration(t *testing.T) {
	if os.Getenv("KOBASHI_TEST_REGISTER_COPILOT") != "1" {
		t.Skip("set KOBASHI_TEST_REGISTER_COPILOT=1 to register the real Kobashi provider without switching any session")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".kobashi", "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	var live session
	if err := json.Unmarshal(data, &live); err != nil || live.Token == "" {
		t.Fatal("connect Kobashi to its authorized GitHub account before this integration check")
	}
	client := newHTTPClient()
	a := testApp(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || (r.URL.Host != "api.githubcopilot.com" && r.URL.Host != "api.github.com") ||
			(r.URL.Path != "/models" && r.URL.Path != "/copilot_internal/v2/token") {
			return nil, errors.New("integration check attempted a request other than token refresh or model discovery")
		}
		return client.Do(r)
	})
	a.state = session{Token: live.Token, Method: live.Method}
	for _, name := range []string{"ws.port", "ws.token"} {
		data, err := os.ReadFile(filepath.Join(home, ".copilot", "run", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := writeAtomic(filepath.Join(a.options.Home, ".copilot", "run", name), data); err != nil {
			t.Fatal(err)
		}
	}
	// Neither fixture can identify a real user session, even if selection regresses.
	for _, selected := range []copilotSelection{{}, {ID: "registration-check-only", Running: 1, Location: "local", Host: "local"}} {
		a.copilotSession = func() (copilotSelection, error) { return selected, nil }
		notice, err := a.enableCopilot()
		if err != nil || !a.snapshot().Copilot || !strings.Contains(notice, "no session was switched") {
			t.Fatal("real provider registration did not complete", notice, err)
		}
	}
}

func TestInstalledCopilotControlReadOnly(t *testing.T) {
	if os.Getenv("KOBASHI_TEST_COPILOT_APP") == "" {
		t.Skip("set KOBASHI_TEST_COPILOT_APP to check the running app without changing it")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	data, err := copilotSessionData(filepath.Join(home, ".copilot", "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	var selected copilotSelection
	if len(data) != 0 {
		if err := json.Unmarshal(data, &selected); err != nil {
			t.Fatal("the app selection has an unsupported response shape", err)
		}
	}
	a, err := New(Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	if _, err := a.bridgedCopilotSessions(); err != nil {
		t.Fatal("could not read dependent sessions without changing them", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	control, err := connectCopilotControl(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer control.conn.CloseNow()
	result, err := control.call(ctx, object{"type": "list_model_providers"}, "model_providers_list")
	if err != nil || len(objectList(result, "providers")) == 0 {
		t.Fatal("could not read the app provider catalog", err)
	}
	result, err = control.call(ctx, object{"type": "list_provider_models", "provider_id": copilotProviderID}, "provider_models_list")
	if err != nil {
		t.Fatal("could not read the app provider model catalog", err)
	}
	if _, ok := result["models"].([]any); !ok {
		t.Fatal("the app provider model catalog has an unsupported response shape")
	}
	if selected.ID != "" && selected.Location == "local" && selected.Host == "local" {
		err := control.sessionIdle(ctx, selected.ID)
		if err != nil && !errors.Is(err, errCopilotSessionBusy) && !errors.Is(err, errCopilotSessionNotReady) {
			t.Fatal("the app does not support a read-only runtime activity check", err)
		}
	}
}
