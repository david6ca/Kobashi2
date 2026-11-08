package bridge

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

func (l *localManager) request(ctx context.Context, path string, body any) (*http.Response, error) {
	l.mu.Lock()
	endpoint, secret := l.endpoint, l.secret
	l.mu.Unlock()
	if endpoint == "" {
		return nil, errors.New("Local model is not running. Select the model account to reconnect")
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+path, bytes.NewReader(jsonBytes(body)))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+secret)
	r.Header.Set("Content-Type", "application/json")
	return l.client.Do(r)
}

const localProbeInstructions = "You are a precise coding assistant. Use the provided tool when requested. Emit a complete tool call in the chat template format, including its closing </tool_call> delimiter."

func (l *localManager) checkModel(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	response, err := l.request(ctx, "/v1/chat/completions", object{
		"model": id, "messages": []any{object{"role": "system", "content": localProbeInstructions}, object{"role": "user", "content": "Call kobashi_ready with value ready."}},
		"tools":       []any{object{"type": "function", "function": object{"name": "kobashi_ready", "description": "Check readiness", "parameters": object{"type": "object", "properties": object{"value": object{"type": "string"}}, "required": []any{"value"}}}}},
		"tool_choice": "required", "max_tokens": 256, "temperature": 0,
	})
	if err != nil {
		return err
	}
	data, err := readResponse(response, 1<<20)
	if err != nil {
		return err
	}
	if response.StatusCode == 507 {
		return errors.New("Not enough available memory to start this model")
	}
	if response.StatusCode != 200 {
		return fmt.Errorf("Local model could not start (HTTP %d)", response.StatusCode)
	}
	var value object
	if json.Unmarshal(data, &value) != nil {
		return errors.New("Local model returned an invalid response")
	}
	for _, choice := range arr(value["choices"]) {
		for _, call := range arr(nested(obj(choice), "message", "tool_calls")) {
			if nested(obj(call), "function", "name") == "kobashi_ready" {
				return nil
			}
		}
	}
	return errors.New("Local model did not complete its tool-call setup. Retry setup")
}
func (l *localManager) proxy(w http.ResponseWriter, r *http.Request, s session, anthropic bool) {
	fail := func(code int, message string) {
		if anthropic {
			claudeError(w, code, message)
		} else {
			respond(w, code, object{"error": object{"message": message, "type": "local_error"}})
		}
	}
	if r.Header.Get("Origin") != "" {
		fail(403, "Browser origins are not allowed")
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		token = r.Header.Get("X-Api-Key")
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte("PROXY_MANAGED")) != 1 {
		fail(401, "Use the Kobashi client configuration")
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
		if anthropic {
			respond(w, 200, object{"data": []any{object{"id": s.LocalModel, "display_name": s.Username, "type": "model", "created_at": timeISO()}}, "has_more": false, "first_id": s.LocalModel, "last_id": s.LocalModel})
		} else {
			respond(w, 200, object{"object": "list", "data": []any{object{"id": s.LocalModel, "object": "model", "owned_by": "local"}}})
		}
		return
	}
	allowed := []string{"/v1/responses", "/v1/responses/compact", "/v1/chat/completions"}
	if anthropic {
		allowed = []string{"/v1/messages", "/v1/messages/count_tokens"}
	}
	if r.Method != http.MethodPost || !contains(allowed, r.URL.Path) {
		fail(404, "Local inference endpoint not found")
		return
	}
	body, err := readObject(r.Body, 256<<20)
	if err != nil {
		fail(400, "Invalid inference request")
		return
	}
	model := str(body["model"])
	if model == "" || (anthropic && strings.HasPrefix(model, "claude-")) {
		body["model"] = s.LocalModel
	} else if model != s.LocalModel {
		fail(400, "Select "+s.Username+" for this conversation")
		return
	}
	var custom localCustomTools
	if !anthropic && strings.HasPrefix(r.URL.Path, "/v1/responses") {
		custom, err = prepareLocalCustomTools(body)
		if err != nil {
			fail(400, err.Error())
			return
		}
	}
	l.mu.Lock()
	endpoint, secret := l.endpoint, l.secret
	l.mu.Unlock()
	if endpoint == "" {
		fail(503, "Local model is not running. Select the model account to reconnect")
		return
	}
	target, err := url.Parse(endpoint)
	if err != nil {
		fail(503, "Local model is unavailable")
		return
	}
	// Never pass client credentials, cookies or proxy settings into the runtime.
	proxy := httputil.ReverseProxy{
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(target)
			p.Out.URL.RawQuery = ""
			p.Out.Header = http.Header{}
			p.Out.Header.Set("Content-Type", "application/json")
			p.Out.Header.Set("Authorization", "Bearer "+secret)
			data := jsonBytes(body)
			p.Out.Body = io.NopCloser(bytes.NewReader(data))
			p.Out.ContentLength = int64(len(data))
		}, Transport: l.client.Transport, FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			if len(custom) > 0 {
				l.app.report(err)
				fail(502, "Local freeform tool response failed: "+err.Error())
				return
			}
			fail(502, "Local inference failed. Select the model account to reconnect")
		},
	}
	if len(custom) > 0 {
		proxy.ModifyResponse = func(response *http.Response) error {
			return l.restoreCustomResponse(r.Context(), response, custom)
		}
	}
	proxy.ServeHTTP(w, r)
}

// Only the marked choice is restored; an intervening user model selection survives.
func restoreLocalModelConfig(live, backup string) string {
	bom, body, nl := sourceParts(live)
	scan := scanToml(body)
	ranges := []textRange{}
	found := false
	for _, a := range scan.assignments {
		if a.table == nil && a.key == "model" && strings.HasSuffix(strings.TrimSpace(body[a.start:a.end]), "# Kobashi local") {
			ranges = append(ranges, a.textRange)
			found = true
		}
	}
	if !found {
		return live
	}
	cleaned := removeRanges(body, ranges)
	_, original, _ := sourceParts(backup)
	for _, a := range scanToml(original).assignments {
		if a.table == nil && a.key == "model" {
			return bom + strings.TrimSpace(original[a.start:a.end]) + nl + cleaned
		}
	}
	return bom + cleaned
}
func injectLocalModelConfig(source, id string) string {
	bom, body, nl := sourceParts(source)
	ranges := []textRange{}
	for _, a := range scanToml(body).assignments {
		if a.table == nil && a.key == "model" {
			ranges = append(ranges, a.textRange)
		}
	}
	return bom + "model = " + string(jsonBytes(id)) + " # Kobashi local" + nl + removeRanges(body, ranges)
}
