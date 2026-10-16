package bridge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const copilotProviderID = "06d32972-e0dd-43fc-a845-4b6f62617368"

var errCopilotNotRunning = errors.New("the Copilot app is not running")
var errCopilotSessionBusy = errors.New("the Copilot session is running; it will connect when idle")
var errCopilotSessionNotReady = errors.New("the Copilot session is not responsive yet; it will connect when ready")

const copilotSessionFieldsSQL = `
 'id',s.id,'model',s.model,'providerId',s.provider_id,
 'running',coalesce(s.is_running,1),'provisional',coalesce(s.provisional,0),'location',s.execution_location,
 'reasoningEffort',s.reasoning_effort,'contextTier',s.context_tier,
 'remote',s.remote_connect_target,
 'host',coalesce((SELECT host_id FROM workspaces WHERE session_id=s.id LIMIT 1),'local')`

const copilotSelectionSQL = `
SELECT json_object(` + copilotSessionFieldsSQL + `)
FROM app_state a
LEFT JOIN workspaces w ON w.id=a.value
JOIN sessions s ON s.id=coalesce(w.session_id,a.value)
WHERE a.key='activeWorkspaceId' AND s.archived_at IS NULL LIMIT 1`

const copilotBridgeSessionsSQL = `
SELECT json_group_array(json_object(` + copilotSessionFieldsSQL + `))
FROM sessions s
WHERE s.provider_id='` + copilotProviderID + `' AND s.archived_at IS NULL`

const copilotLocalSessionsSQL = `
SELECT json_group_array(json_object(` + copilotSessionFieldsSQL + `))
FROM sessions s
WHERE s.archived_at IS NULL AND s.execution_location='local'
 AND coalesce(s.provisional,0)=0
 AND coalesce(s.remote_connect_target,'')=''
 AND NOT EXISTS (SELECT 1 FROM workspaces w WHERE w.session_id=s.id AND coalesce(w.host_id,'local')!='local')`

type copilotSelection struct {
	ID              string `json:"id"`
	Model           string `json:"model"`
	ProviderID      string `json:"providerId"`
	ReasoningEffort string `json:"reasoningEffort"`
	ContextTier     string `json:"contextTier"`
	Running         int    `json:"running"`
	Provisional     int    `json:"provisional"`
	Location        string `json:"location"`
	Remote          string `json:"remote"`
	Host            string `json:"host"`
}

func copilotSessionData(path string) ([]byte, error) {
	return copilotQuery(path, copilotSelectionSQL)
}

func (a *App) bridgedCopilotSessions() ([]copilotSelection, error) {
	if a.copilotSessions != nil {
		return a.copilotSessions()
	}
	data, err := copilotQuery(filepath.Join(a.options.Home, ".copilot", "data.db"), copilotBridgeSessionsSQL)
	if errors.Is(err, os.ErrNotExist) {
		saved, restoreErr := a.copilotRestoreSelections()
		if restoreErr == nil && len(saved) == 0 {
			return nil, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("read Copilot sessions before disabling the bridge: %w", err)
	}
	return decodeCopilotSessions(data)
}

func decodeCopilotSessions(data []byte) ([]copilotSelection, error) {
	var sessions []copilotSelection
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("invalid Copilot sessions: %w", err)
	}
	return sessions, nil
}

func copilotWireModel(providerID, model string) string {
	if providerID != "" {
		return strings.TrimPrefix(model, providerID+"/")
	}
	return model
}

func (s copilotSelection) customProvider() bool {
	return s.ProviderID != "" && s.ProviderID != copilotProviderID && !strings.HasPrefix(s.ProviderID, "github_copilot:")
}

func (a *App) selectedCopilotSession() (copilotSelection, error) {
	if a.copilotSession != nil {
		return a.copilotSession()
	}
	var selected copilotSelection
	data, err := copilotSessionData(filepath.Join(a.options.Home, ".copilot", "data.db"))
	if err != nil {
		return selected, fmt.Errorf("read the selected Copilot session: %w", err)
	}
	if len(data) != 0 {
		if err = json.Unmarshal(data, &selected); err != nil {
			return selected, fmt.Errorf("invalid Copilot session selection: %w", err)
		}
	}
	return selected, nil
}

func (s copilotSelection) ready() error {
	if s.ID == "" {
		return errors.New("no local Copilot session is available yet")
	}
	if s.Location != "local" || s.Remote != "" || s.Host != "local" {
		return errors.New("Copilot Bridge can only select a model for a local Copilot session")
	}
	if s.Running != 0 {
		return errCopilotSessionBusy
	}
	if s.Provisional != 0 {
		return errors.New("the Copilot session is still being prepared; it will connect when ready")
	}
	return nil
}

func (a *App) copilotRestoreSelections() (map[string]copilotSelection, error) {
	saved := map[string]copilotSelection{}
	data, err := readOptional(a.path("copilot-restore.json"))
	if err != nil {
		return nil, err
	}
	if data != nil {
		if err = json.Unmarshal(data, &saved); err != nil || saved == nil {
			return nil, errors.New("invalid saved Copilot session selections")
		}
		for id, selection := range saved {
			if id == "" || selection.ID != id || selection.Model == "" || selection.ProviderID == copilotProviderID {
				return nil, errors.New("invalid saved Copilot session selection")
			}
		}
	}
	return saved, nil
}

func (a *App) restoreCopilot() error {
	sessions, err := a.bridgedCopilotSessions()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return nil
	}
	saved, err := a.copilotRestoreSelections()
	if err != nil {
		return err
	}
	for _, selected := range sessions {
		if err = selected.ready(); err != nil {
			return fmt.Errorf("Copilot Bridge remains enabled; wait for its local sessions to be idle before disabling it: %w", err)
		}
		if _, ok := saved[selected.ID]; !ok {
			return errors.New("Copilot Bridge remains enabled; a session has no saved original provider. Select a non-Kobashi model in that session before disabling the bridge")
		}
	}
	ctx, cancel := context.WithTimeout(a.ctx, 40*time.Second)
	defer cancel()
	control, err := connectCopilotControl(ctx, a.options.Home)
	if err != nil {
		return err
	}
	defer control.conn.CloseNow()
	for _, selected := range sessions {
		current, err := a.bridgedCopilotSessions()
		if err != nil {
			return err
		}
		for _, candidate := range current {
			if candidate.ID != selected.ID {
				continue
			}
			if candidate != selected {
				return errors.New("Copilot Bridge remains enabled; a session changed while restoring its original provider. Try disabling again when idle")
			}
			if err = control.sessionIdle(ctx, selected.ID); err != nil {
				return fmt.Errorf("Copilot Bridge remains enabled; could not confirm the session is idle before restoring its original provider: %w", err)
			}
			latest, err := a.bridgedCopilotSessions()
			if err != nil {
				return err
			}
			unchanged := false
			for _, session := range latest {
				if session == selected {
					unchanged = true
					break
				}
			}
			if !unchanged {
				return errors.New("Copilot Bridge remains enabled; a session changed while checking its runtime. Try disabling again when idle")
			}
			if err = control.selectModel(ctx, saved[selected.ID]); err != nil {
				if copilotControlInterrupted(err) {
					log.Printf("Copilot restore confirmation interrupted for session %s: %v", selected.ID, err)
					return errors.New("Copilot disconnected during restore. Bridge stays enabled; when idle, select a non-Kobashi model before disabling it")
				}
				return fmt.Errorf("Copilot Bridge remains enabled; could not restore a session's original provider: %w", err)
			}
			break
		}
	}
	remaining, err := a.bridgedCopilotSessions()
	if err != nil {
		return err
	}
	if len(remaining) != 0 {
		return errors.New("Copilot Bridge remains enabled; some sessions still use Kobashi")
	}
	return nil
}

type copilotControl struct {
	conn     *websocket.Conn
	token    string
	instance string
}

func (c *copilotControl) sessionIdle(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := c.call(ctx, object{"type": "probe_session_liveness", "session_id": id, "recover": false}, "session_liveness_result")
	if err != nil {
		return err
	}
	running, ok := result["is_running"].(bool)
	if result["session_id"] != id || !ok || str(result["status"]) == "" {
		return errors.New("Copilot app did not confirm the session's runtime activity")
	}
	if running {
		return errCopilotSessionBusy
	}
	if result["status"] != "responsive" && result["status"] != "idle_without_runtime" {
		return errCopilotSessionNotReady
	}
	return nil
}

type copilotControlTransportError struct {
	operation string
	detail    string
	cause     error
}

func (e *copilotControlTransportError) Error() string {
	return fmt.Sprintf("%s: %s", e.operation, e.detail)
}

func (e *copilotControlTransportError) Unwrap() error { return e.cause }

func copilotControlInterrupted(err error) bool {
	var transport *copilotControlTransportError
	return errors.As(err, &transport)
}

func (c *copilotControl) selectModel(ctx context.Context, selected copilotSelection) error {
	tier := selected.ContextTier
	if tier == "" {
		tier = "default"
	}
	command := object{"type": "set_session_model", "session_id": selected.ID,
		"provider_id": selected.ProviderID, "model": copilotWireModel(selected.ProviderID, selected.Model), "context_tier": tier}
	if selected.ProviderID == "" {
		command["provider_id"] = nil
	}
	if selected.ReasoningEffort != "" {
		command["reasoning_effort"] = selected.ReasoningEffort
	}
	result, err := c.call(ctx, command, "session_model_result")
	if err != nil {
		return err
	}
	providerID, model := str(result["provider_id"]), str(result["model"])
	providerMatches := providerID == selected.ProviderID ||
		(selected.ProviderID == "" && strings.HasPrefix(providerID, "github_copilot:"))
	if result["session_id"] != selected.ID || !providerMatches ||
		copilotWireModel(providerID, model) != command["model"] {
		return errors.New("Copilot app did not confirm the requested provider and model")
	}
	return nil
}

func copilotRunValue(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	lines := strings.Fields(string(data))
	if len(lines) != 2 {
		return "", "", errors.New("invalid Copilot app control file")
	}
	pid, err := strconv.Atoi(lines[1])
	if err != nil || pid <= 0 {
		return "", "", errors.New("invalid Copilot app process identifier")
	}
	return lines[0], lines[1], nil
}

func connectCopilotControl(ctx context.Context, home string) (*copilotControl, error) {
	run := filepath.Join(home, ".copilot", "run")
	port, owner, err := copilotRunValue(filepath.Join(run, "ws.port"))
	if err != nil {
		return nil, fmt.Errorf("open the GitHub Copilot app before enabling Copilot Bridge: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return nil, errors.New("invalid Copilot app control port")
	}
	token, tokenOwner, err := copilotRunValue(filepath.Join(run, "ws.token"))
	if err != nil {
		return nil, fmt.Errorf("read Copilot app control authentication: %w", err)
	}
	if owner != tokenOwner {
		return nil, fmt.Errorf("%w: Copilot app control files refer to different processes; waiting for startup", errCopilotNotRunning)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("Copilot app control redirects are not allowed")
	}}
	conn, _, err := websocket.Dial(ctx, "ws://127.0.0.1:"+port+"?token="+url.QueryEscape(token), &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		if copilotConnectionRefused(err) {
			return nil, fmt.Errorf("%w: %s", errCopilotNotRunning, redactCopilotControlError(err, token))
		}
		return nil, fmt.Errorf("connect to the GitHub Copilot app: %s", redactCopilotControlError(err, token))
	}
	conn.SetReadLimit(16 << 20)
	return &copilotControl{conn: conn, token: token, instance: port + "/" + owner}, nil
}

func redactCopilotControlError(err error, token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(err.Error(), url.QueryEscape(token), "[redacted]"), token, "[redacted]")
}

func (c *copilotControl) call(ctx context.Context, command object, responseType string) (object, error) {
	id := rand.Text()
	command["request_id"] = id
	if err := wsjson.Write(ctx, c.conn, command); err != nil {
		return nil, &copilotControlTransportError{fmt.Sprintf("send Copilot app command %s", command["type"]), redactCopilotControlError(err, c.token), err}
	}
	var confirmed object
	swapping, swapped := false, false
	for {
		var message object
		if err := wsjson.Read(ctx, c.conn, &message); err != nil {
			return nil, &copilotControlTransportError{fmt.Sprintf("read Copilot app response to %s", command["type"]), redactCopilotControlError(err, c.token), err}
		}
		requestID := str(message["request_id"])
		if requestID != "" && requestID != id {
			continue
		}
		if requestID == "" && (responseType == "model_providers_list" || responseType == "provider_models_list" || responseType == "session_liveness_result") {
			continue
		}
		if command["type"] == "set_session_model" && message["session_id"] == command["session_id"] {
			switch message["type"] {
			case "session_provider_swap_started":
				swapping = true
				continue
			case "session_provider_swap_completed":
				if detail := str(message["error"]); detail != "" {
					return nil, fmt.Errorf("Copilot app: %s", redactCopilotControlError(errors.New(detail), c.token))
				}
				swapped = true
				if confirmed != nil {
					return confirmed, nil
				}
				continue
			}
		}
		if requestID == id && (message["type"] == "error" || str(message["error"]) != "") {
			detail := str(message["message"])
			if detail == "" {
				detail = str(message["error"])
			}
			if detail == "" {
				detail = "command rejected"
			}
			detail = redactCopilotControlError(errors.New(detail), c.token)
			return nil, fmt.Errorf("Copilot app: %s", detail)
		}
		if message["type"] == responseType {
			if responseType == "model_provider_upserted" && nested(message, "provider", "id") != nested(command, "provider", "id") {
				continue
			}
			if responseType == "provider_model_upserted" &&
				(nested(message, "model", "providerId") != nested(command, "model", "providerId") ||
					nested(message, "model", "modelId") != nested(command, "model", "modelId")) {
				continue
			}
			if responseType == "session_model_result" && message["session_id"] != command["session_id"] {
				continue
			}
			if responseType == "session_model_result" && swapping && !swapped {
				confirmed = message
				continue
			}
			return message, nil
		}
	}
}

func copilotSetupModel(models []object, preferred string) (object, string, error) {
	var first object
	for _, model := range models {
		id := str(model["id"])
		policy := str(nested(model, "policy", "state"))
		kind := str(nested(model, "capabilities", "type"))
		endpoints := stringsOf(model["supported_endpoints"])
		if id == "" || (policy != "" && policy != "enabled") || (kind != "" && kind != "chat") ||
			(!contains(endpoints, "/responses") && !contains(endpoints, "/chat/completions")) {
			continue
		}
		if first == nil {
			first = model
		}
		if id == preferred {
			first = model
			break
		}
	}
	if first == nil {
		return nil, "", errors.New("the Kobashi account has no compatible Copilot app chat models")
	}
	wire := "completions"
	if contains(stringsOf(first["supported_endpoints"]), "/responses") {
		wire = "responses"
	}
	return first, wire, nil
}

func (a *App) enableCopilot() (string, error) {
	a.copilotRouting = copilotRouting{explicit: true}
	selected, selectionErr := a.selectedCopilotSession()
	sessions, err := a.localCopilotSessions()
	if err == nil || errors.Is(err, os.ErrNotExist) {
		a.copilotRouting.known = map[string]copilotSelection{}
		a.copilotRouting.pending = map[string]bool{}
		a.copilotRouting.handled = map[string]bool{}
		for _, session := range sessions {
			a.copilotRouting.known[session.ID] = session
			if session.ID != selected.ID && session.customProvider() {
				a.copilotRouting.handled[session.ID] = true
			}
		}
		if _, ok := a.copilotRouting.known[selected.ID]; ok {
			a.copilotRouting.pending[selected.ID] = true
		}
	} else {
		log.Printf("Copilot automatic connection will retry session inspection: %v", err)
	}
	return a.configureCopilot(selected, selectionErr, a.selectedCopilotSession)
}

func (a *App) configureCopilot(selected copilotSelection, selectionErr error, currentSelection func() (copilotSelection, error)) (string, error) {
	previous := a.snapshot().Copilot
	a.mu.Lock()
	a.state.Copilot = true
	a.mu.Unlock()
	if err := a.saveSession(); err != nil {
		a.mu.Lock()
		a.state.Copilot = previous
		a.mu.Unlock()
		return "", err
	}
	rollback := func(cause error) (string, error) {
		a.mu.Lock()
		a.state.Copilot = previous
		a.mu.Unlock()
		if saveErr := a.saveSession(); saveErr != nil {
			a.mu.Lock()
			a.state.Copilot = true
			a.mu.Unlock()
			return "", errors.Join(cause, fmt.Errorf("Copilot Bridge remains enabled; could not save its disabled state: %w", saveErr))
		}
		return "", cause
	}
	ctx, cancel := context.WithTimeout(a.ctx, 40*time.Second)
	defer cancel()
	control, err := connectCopilotControl(ctx, a.options.Home)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, errCopilotNotRunning) {
			return "Copilot is enabled and will connect when the app starts.", nil
		}
		return rollback(err)
	}
	defer control.conn.CloseNow()
	result, err := control.call(ctx, object{"type": "list_model_providers"}, "model_providers_list")
	if err != nil {
		return rollback(err)
	}
	providers, ok := result["providers"].([]any)
	if !ok {
		return rollback(errors.New("Copilot app returned an invalid provider catalog"))
	}
	settings := object{}
	var registeredProvider object
	for _, item := range providers {
		provider := obj(item)
		if str(provider["id"]) == "" {
			return rollback(errors.New("Copilot app returned an invalid provider"))
		}
		if provider["id"] != copilotProviderID {
			continue
		}
		settings = obj(provider["settings"])
		if provider["kind"] != "custom" || provider["name"] != "Kobashi" || settings["managedBy"] != "Kobashi" {
			return rollback(errors.New("the Kobashi provider identifier is already used by another configuration; it was not changed"))
		}
		settings = clone(settings)
		registeredProvider = provider
	}
	models, err := a.discover(ctx, false)
	if err != nil {
		return rollback(err)
	}
	selectedModel := copilotWireModel(selected.ProviderID, selected.Model)
	model, wire, err := copilotSetupModel(models, selectedModel)
	if err != nil {
		return rollback(err)
	}
	result, err = control.call(ctx, object{"type": "list_provider_models", "provider_id": copilotProviderID}, "provider_models_list")
	if err != nil {
		return rollback(err)
	}
	entries, ok := result["models"].([]any)
	if !ok {
		return rollback(errors.New("Copilot app returned an invalid provider model catalog"))
	}
	modelID := ""
	var registeredModel object
	for _, item := range entries {
		existing := obj(item)
		if existing["providerId"] == copilotProviderID && existing["modelId"] == model["id"] {
			modelID = str(existing["id"])
			if modelID == "" {
				return rollback(errors.New("Copilot app returned a model without its registration identifier"))
			}
			registeredModel = existing
			break
		}
	}
	providerWire := str(settings["wireApi"])
	if providerWire != "responses" && providerWire != "completions" {
		providerWire = wire
	}
	settings["baseUrl"], settings["wireApi"] = a.CopilotURL(), providerWire
	settings["authKind"], settings["managedBy"] = "api_key", "Kobashi"
	settings["headersJson"] = "{}"
	if registeredProvider["hasSecret"] != true || !bytes.Equal(jsonBytes(registeredProvider["settings"]), jsonBytes(settings)) {
		if _, err = control.call(ctx, object{"type": "upsert_model_provider",
			"provider": object{"id": copilotProviderID, "name": "Kobashi", "kind": "custom", "settings": settings},
			"secret":   object{"kind": "apiKey", "value": "PROXY_MANAGED"}}, "model_provider_upserted"); err != nil {
			return rollback(err)
		}
	}
	name := str(model["name"])
	if name == "" {
		name = str(model["id"])
	}
	entry := object{"providerId": copilotProviderID, "modelId": model["id"], "wireModel": model["id"],
		"displayName": name, "wireApiOverride": wire}
	if modelID != "" {
		entry["id"] = modelID
	}
	for key, field := range map[string]string{"max_prompt_tokens": "maxPromptTokens", "max_output_tokens": "maxOutputTokens"} {
		if value := number(nested(model, "capabilities", "limits", key)); value > 0 {
			entry[field] = value
		}
	}
	efforts := stringsOf(nested(model, "capabilities", "supports", "reasoning_effort"))
	if len(efforts) > 0 {
		entry["supportedReasoningEfforts"] = efforts
	}
	modelMatches := registeredModel != nil
	for key, value := range entry {
		if !bytes.Equal(jsonBytes(registeredModel[key]), jsonBytes(value)) {
			modelMatches = false
			break
		}
	}
	if !modelMatches {
		if _, err = control.call(ctx, object{"type": "upsert_provider_model", "model": entry}, "provider_model_upserted"); err != nil {
			return rollback(err)
		}
	}
	result, err = control.call(ctx, object{"type": "list_model_providers"}, "model_providers_list")
	if err != nil {
		return rollback(err)
	}
	verified := false
	providers, ok = result["providers"].([]any)
	if ok {
		for _, item := range providers {
			provider := obj(item)
			if provider["id"] == copilotProviderID {
				saved := obj(provider["settings"])
				verified = provider["name"] == "Kobashi" && provider["kind"] == "custom" && provider["hasSecret"] == true &&
					saved["baseUrl"] == a.CopilotURL() && saved["wireApi"] == providerWire && saved["authKind"] == "api_key" &&
					saved["managedBy"] == "Kobashi"
			}
		}
	}
	if !verified {
		return rollback(errors.New("Copilot app did not persist the Kobashi provider and its credential"))
	}
	result, err = control.call(ctx, object{"type": "list_provider_models", "provider_id": copilotProviderID}, "provider_models_list")
	if err != nil {
		return rollback(err)
	}
	verified = false
	entries, ok = result["models"].([]any)
	if ok {
		for _, item := range entries {
			saved := obj(item)
			if saved["providerId"] == copilotProviderID && saved["modelId"] == model["id"] &&
				str(saved["id"]) != "" && saved["wireApiOverride"] == wire {
				verified = true
			}
		}
	}
	if !verified {
		return rollback(errors.New("Copilot app did not persist the Kobashi provider model"))
	}
	a.copilotRouting.instance = control.instance
	pending := func(reason error) (string, error) {
		log.Printf("Copilot provider registered without switching a session: %v", reason)
		return "Kobashi registered; no session was switched. Local sessions connect automatically when idle.", nil
	}
	if selectionErr != nil {
		return pending(selectionErr)
	}
	if err = selected.ready(); err != nil {
		return pending(err)
	}
	if selected.ProviderID == copilotProviderID {
		if a.copilotRouting.handled == nil {
			a.copilotRouting.handled = map[string]bool{}
		}
		a.copilotRouting.handled[selected.ID] = true
		return "", nil
	}
	if err = control.sessionIdle(ctx, selected.ID); err != nil {
		if errors.Is(err, errCopilotSessionBusy) || errors.Is(err, errCopilotSessionNotReady) {
			if errors.Is(err, errCopilotSessionNotReady) {
				if a.copilotRouting.retryAt == nil {
					a.copilotRouting.retryAt = map[string]time.Time{}
				}
				a.copilotRouting.retryAt[selected.ID] = time.Now().Add(5 * time.Second)
			}
			return pending(err)
		}
		return "", fmt.Errorf("Copilot Bridge remains enabled; could not verify that the session is idle: %w", err)
	}
	target := copilotSelection{ID: selected.ID, ProviderID: copilotProviderID, Model: str(model["id"]), ContextTier: "default"}
	if model["id"] == selectedModel && contains(efforts, selected.ReasoningEffort) {
		target.ReasoningEffort = selected.ReasoningEffort
	}
	current, err := currentSelection()
	if err != nil {
		return pending(err)
	}
	if err = current.ready(); err != nil {
		return pending(err)
	}
	if current.ID != selected.ID || current.Model != selected.Model || current.ProviderID != selected.ProviderID ||
		current.ReasoningEffort != selected.ReasoningEffort || current.ContextTier != selected.ContextTier {
		return pending(errors.New("the selected Copilot session or model changed during setup"))
	}
	if selected.Model == "" {
		return pending(errors.New("the selected Copilot session has no model to restore"))
	}
	if selected.ProviderID != copilotProviderID {
		saved, saveErr := a.copilotRestoreSelections()
		if saveErr != nil {
			return "", saveErr
		}
		saved[selected.ID] = selected
		if saveErr = writeJSON(a.path("copilot-restore.json"), saved); saveErr != nil {
			return "", fmt.Errorf("save the original Copilot session before switching providers: %w", saveErr)
		}
	}
	if a.copilotRouting.handled == nil {
		a.copilotRouting.handled = map[string]bool{}
	}
	// Do not retry an uncertain swap or override a later manual model selection.
	a.copilotRouting.handled[selected.ID] = true
	err = control.selectModel(ctx, target)
	if err != nil {
		// A lost acknowledgement does not cancel an accepted app command.
		if copilotControlInterrupted(err) {
			log.Printf("Copilot model switch confirmation interrupted for session %s: %v", selected.ID, err)
			return "", errors.New("Copilot disconnected during the model switch. Bridge stays enabled; when idle, select Kobashi in Copilot's model picker")
		}
		return "", fmt.Errorf("Kobashi provider registered and Copilot Bridge remains enabled; could not confirm the model switch: %w", err)
	}
	return "Kobashi provider registered and selected for the current local session.", nil
}
