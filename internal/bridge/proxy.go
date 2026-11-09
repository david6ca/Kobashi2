package bridge

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func (a *App) codex(w http.ResponseWriter, r *http.Request) {
	w, r, done := logProxyRequest(w, r, "Codex")
	defer done()
	a.openAI(w, r, false)
}
func (a *App) copilotApp(w http.ResponseWriter, r *http.Request) {
	w, r, done := logProxyRequest(w, r, "Copilot")
	defer done()
	if r.Header.Get("Origin") != "" {
		respond(w, 403, object{"error": "Browser origins are not allowed"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer PROXY_MANAGED")) != 1 {
		respond(w, 401, object{"error": "Set the Kobashi provider API key to PROXY_MANAGED"})
		return
	}
	a.openAI(w, r, true)
}
func (a *App) openAI(w http.ResponseWriter, r *http.Request, copilotApp bool) {
	a.routes.RLock()
	defer a.routes.RUnlock()
	state := a.snapshot()
	name, enabled := "Codex", state.Codex
	// protocol := "codex"
	if copilotApp {
		name, enabled = "Copilot", state.Copilot
		// protocol = "copilot"
		if !((r.Method == "GET" && r.URL.Path == "/v1/models") ||
			(r.Method == "POST" && contains([]string{"/v1/responses", "/v1/responses/compact", "/v1/chat/completions"}, r.URL.Path))) {
			respond(w, 404, object{"error": "Inference endpoint not found"})
			return
		}
	}
	/*
		if state.RemoteURL != "" {
			a.forwardRemote(w, r, state, protocol, enabled)
			return
		}
	*/
	if !state.active() || !enabled {
		respond(w, 401, object{"error": name + " bridge not active"})
		return
	}
	if state.LocalModel != "" {
		a.local.proxy(w, r, state, false)
		return
	}
	if !copilotApp && contains([]string{"/v1/responses", "/v1/responses/compact"}, r.URL.Path) && r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		log.Printf("request=%s client=Codex rejected reason=method_not_allowed websocket_upgrade=%t", requestLogID(r.Context()), strings.EqualFold(r.Header.Get("Upgrade"), "websocket"))
		respond(w, http.StatusMethodNotAllowed, object{"error": object{"code": "method_not_allowed", "message": "Use POST for this endpoint; WebSocket is not supported."}})
		return
	}
	if r.Method == "GET" && r.URL.Path == "/v1/models" {
		if copilotApp {
			raw, err := a.discover(r.Context(), false)
			if err != nil {
				respond(w, 502, object{"error": err.Error()})
				return
			}
			out := []object{}
			for _, model := range raw {
				if str(model["id"]) != "" {
					value := clone(model)
					value["object"] = "model"
					value["created"] = number(model["created"])
					owner := str(model["vendor"])
					if owner == "" {
						owner = "copilot"
					}
					value["owned_by"] = owner
					out = append(out, value)
				}
			}
			respond(w, 200, object{"object": "list", "data": out})
			return
		}
		if err := a.models.ensureSession(r.Context()); err != nil {
			log.Printf("request=%s client=Codex model_startup_error=%q", requestLogID(r.Context()), diagnosticError(err))
		}
		out := []any{}
		for _, m := range a.models.visible() {
			vendor := strings.ToLower(str(m["vendor"]))
			if vendor == "" {
				vendor = "copilot"
			}
			vendor = strings.Join(strings.Fields(vendor), "-")
			out = append(out, object{"id": m["id"], "object": "model", "created": number(m["created"]), "owned_by": vendor})
		}
		respond(w, 200, object{"object": "list", "data": out})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<20+1))
	if err != nil {
		respond(w, 400, object{"error": err.Error()})
		return
	}
	if len(body) > 256<<20 {
		log.Printf("request=%s client=%s rejected reason=request_too_large source=local request_bytes_at_least=%d limit_bytes=%d", requestLogID(r.Context()), name, len(body), 256<<20)
		respond(w, 413, object{"error": "Request exceeds 256 MiB"})
		return
	}
	requestBytes := len(body)
	var requestBody object
	var metadata struct {
		Model  string
		Stream bool
	}
	if json.Unmarshal(body, &metadata) == nil {
		log.Printf("request=%s model=%q stream=%t", requestLogID(r.Context()), metadata.Model, metadata.Stream)
	}
	token := ""
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if r.Method == "POST" && strings.Contains(r.URL.Path, "/responses") && decoder.Decode(&requestBody) == nil && requestBody != nil && decoder.Decode(new(any)) == io.EOF {
		if !copilotApp {
			if err := a.models.ensureSession(r.Context()); err != nil {
				log.Printf("request=%s client=Codex capability_lookup_error=%q", requestLogID(r.Context()), diagnosticError(err))
			}
			rules, t := a.models.requestRules(str(requestBody["model"]))
			token = t
			if normalized, changed := applyConstraints(requestBody, rules); changed {
				requestBody = normalized
				body = jsonBytes(normalized)
			}
		}
		if changed := a.pngPayloads.optimize(requestBody); changed > 0 {
			before := len(body)
			body = jsonBytes(requestBody)
			log.Printf("request=%s client=%s lossless_png images=%d request_bytes=%d upstream_request_bytes=%d saved_bytes=%d", requestLogID(r.Context()), name, changed, before, len(body), before-len(body))
		}
	}
	path := r.URL.RequestURI()
	if !strings.HasPrefix(path, "/v1") {
		path = "/v1" + path
	}
	if copilotApp && r.URL.Path == "/v1/chat/completions" {
		path = "/chat/completions"
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
	}
	authRetried := false
	for attempt := 0; attempt < 4; attempt++ {
		response, err := a.copilotWithRetry(r.Context(), r.Method, path, body, &authRetried)
		if err != nil {
			respond(w, 502, object{"error": err.Error()})
			return
		}
		if response.StatusCode == http.StatusRequestEntityTooLarge {
			data, e := readResponse(response, 16<<20)
			if e != nil {
				respond(w, 502, object{"error": e.Error()})
				return
			}
			var errorBody object
			_ = json.Unmarshal(data, &errorBody)
			detail := clone(parseError(errorBody))
			if detail == nil {
				detail = object{"message": str(errorBody["error"])}
			}
			log.Printf("request=%s client=%s upstream_failure status=413 model=%q request_bytes=%d upstream_request_bytes=%d code=%q message=%q", requestLogID(r.Context()), name, metadata.Model, requestBytes, len(body), str(detail["code"]), str(detail["message"]))
			// Codex retries HTTP 413 even with X-Should-Retry: false, but treats
			// HTTP 400 as a terminal request error. Keep the original status in
			// diagnostics and headers; do not teach model rules or replay the body.
			if parseError(errorBody) != nil || str(errorBody["error"]) != "" {
				message := "Request too large for GitHub Copilot. Reduce attachments or shorten the conversation."
				if upstreamMessage := str(detail["message"]); upstreamMessage != "" {
					message += " Upstream: " + upstreamMessage
				}
				detail["message"] = message
				if str(detail["code"]) == "" {
					detail["code"] = "request_too_large"
				}
				if str(detail["type"]) == "" {
					detail["type"] = "invalid_request_error"
				}
				errorBody["error"] = detail
				data = jsonBytes(errorBody)
				response.Header.Set("Content-Type", "application/json")
				response.Header.Del("Content-Length")
				response.Header.Del("Content-Encoding")
			}
			response.Body = io.NopCloser(bytes.NewReader(data))
			response.Header.Set("X-Should-Retry", "false")
			if !copilotApp {
				response.StatusCode = http.StatusBadRequest
				response.Header.Set("X-Kobashi-Upstream-Status", "413")
				log.Printf("request=%s client=Codex translated_error upstream_status=413 downstream_status=400 reason=request_too_large", requestLogID(r.Context()))
			}
		} else if response.StatusCode == 400 || response.StatusCode == 403 || response.StatusCode == 404 || response.StatusCode == 410 {
			data, e := readResponse(response, 16<<20)
			if e != nil {
				respond(w, 502, object{"error": e.Error()})
				return
			}
			var errorBody object
			_ = json.Unmarshal(data, &errorBody)
			if detail := parseError(errorBody); detail != nil {
				log.Printf("request=%s client=%s upstream_failure status=%d model=%q code=%q message=%q", requestLogID(r.Context()), name, response.StatusCode, metadata.Model, str(detail["code"]), str(detail["message"]))
			}
			if !copilotApp {
				a.models.observe(str(requestBody["model"]), token, response.StatusCode, errorBody, nil)
			}
			if response.StatusCode == 400 && attempt < 3 && requestBody != nil {
				next, rules, changed := repairRequest(requestBody, errorBody)
				if changed {
					log.Printf("request=%s client=%s retry reason=compatibility model=%q attempt=%d max_retries=3 rules=%v", requestLogID(r.Context()), name, str(requestBody["model"]), attempt+1, rules)
					if !copilotApp {
						a.models.observe(str(requestBody["model"]), token, response.StatusCode, errorBody, rules)
					}
					requestBody = next
					body = jsonBytes(next)
					continue
				}
			}
			contentType := response.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/json"
			}
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(response.StatusCode)
			_, _ = w.Write(data)
			return
		}
		normalize := response.StatusCode == 200 && (copilotApp || strings.Contains(path, "/responses") || strings.Contains(path, "/chat/completions")) && strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") &&
			(response.Header.Get("Content-Encoding") == "" || response.Header.Get("Content-Encoding") == "identity")
		for key, values := range response.Header {
			if strings.EqualFold(key, "Connection") || strings.EqualFold(key, "Transfer-Encoding") || (normalize && strings.EqualFold(key, "Content-Length")) {
				continue
			}
			w.Header()[key] = values
		}
		w.WriteHeader(response.StatusCode)
		if normalize {
			failureLogged := false
			terminal, lastEvent, responseID, events := "", "", "", 0
			err = normalizeSSE(response.Body, w, func(event object) {
				events++
				if responseID == "" {
					responseID = str(nested(event, "response", "id"))
					if responseID == "" {
						responseID = str(event["response_id"])
					}
				}
				lastEvent = str(event["type"])
				if contains([]string{"response.completed", "response.failed", "response.incomplete", "error"}, lastEvent) {
					terminal = lastEvent
				}
				for _, choice := range arr(event["choices"]) {
					if str(obj(choice)["finish_reason"]) != "" {
						terminal = "chat.finish"
					}
				}
				detail := parseError(event)
				if detail == nil {
					return
				}
				if !failureLogged {
					log.Printf("request=%s client=%s upstream_failure model=%q code=%q message=%q", requestLogID(r.Context()), name, metadata.Model, str(detail["code"]), str(detail["message"]))
					failureLogged = true
				}
				if requestBody == nil || copilotApp {
					return
				}
				_, rules, _ := repairRequest(requestBody, event)
				a.models.observe(str(requestBody["model"]), token, 400, event, rules)
			})
			outcome := terminal
			if outcome == "" {
				outcome = "eof_without_terminal"
			}
			if err != nil {
				outcome = "io_error"
			}
			if r.Context().Err() != nil {
				outcome = "canceled"
			}
			log.Printf("request=%s client=%s stream_end outcome=%s terminal=%q last_event=%q events=%d error=%q context_error=%q",
				requestLogID(r.Context()), name, outcome, terminal, lastEvent, events, diagnosticError(err), diagnosticError(r.Context().Err()))
			tracked, _ := w.(*diagnosticResponse)
			if terminal == "" && r.Context().Err() == nil && (tracked == nil || tracked.writeErr == nil) {
				code, message := "upstream_stream_interrupted", "The upstream connection closed before the response finished."
				var timeout interface{ Timeout() bool }
				if errors.As(err, &timeout) && timeout.Timeout() {
					code, message = "upstream_timeout", "The upstream provider stopped sending data."
				}
				kind := "response.failed"
				event := object{"type": kind, "response": object{"id": responseID, "object": "response", "status": "failed", "output": []any{}, "error": object{"code": code, "message": message}}}
				if strings.Contains(path, "/chat/completions") {
					kind = "error"
					event = object{"error": object{"type": "server_error", "code": code, "message": message, "param": nil}}
				}
				// Complete any truncated frame before sending a distinct error.
				_, writeErr := io.WriteString(w, "\n\nevent: "+kind+"\ndata: "+string(jsonBytes(event))+"\n\n")
				if writeErr == nil {
					writeErr = http.NewResponseController(w).Flush()
				}
				if writeErr != nil {
					log.Printf("request=%s client=%s stream_error_delivery=%q", requestLogID(r.Context()), name, diagnosticError(writeErr))
				}
			}
		} else {
			_, err = io.Copy(w, response.Body)
		}
		response.Body.Close()
		if err != nil {
			log.Printf("request=%s client=%s upstream_stream_error=%q", requestLogID(r.Context()), name, diagnosticError(err))
		}
		return
	}
}
func claudeError(w http.ResponseWriter, status int, message string) {
	typ := map[int]string{400: "invalid_request_error", 401: "authentication_error", 403: "permission_error", 404: "not_found_error", 429: "rate_limit_error"}[status]
	if typ == "" {
		typ = "api_error"
	}
	respond(w, status, object{"type": "error", "error": object{"type": typ, "message": message}})
}
func (a *App) claude(w http.ResponseWriter, r *http.Request) {
	w, r, done := logProxyRequest(w, r, "Claude")
	defer done()
	a.routes.RLock()
	defer a.routes.RUnlock()
	state := a.snapshot()
	/*
		if state.RemoteURL != "" {
			a.forwardRemote(w, r, state, "claude", state.Claude)
			return
		}
	*/
	if !state.active() || !state.Claude {
		claudeError(w, 401, "Claude bridge not active")
		return
	}
	if state.LocalModel != "" {
		a.local.proxy(w, r, state, true)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/v1/models" {
		raw, err := a.discover(r.Context(), false)
		if err != nil {
			claudeError(w, 502, err.Error())
			return
		}
		models := claudeModels(raw)
		out := []any{}
		var first, last any
		for _, m := range models {
			delete(m, "copilot")
			m["type"] = "model"
			m["created_at"] = timeISO()
			out = append(out, m)
		}
		if len(models) > 0 {
			first = models[0]["id"]
			last = models[len(models)-1]["id"]
		}
		respond(w, 200, object{"data": out, "has_more": false, "first_id": first, "last_id": last})
		return
	}
	if r.Method != "POST" || (r.URL.Path != "/v1/messages" && r.URL.Path != "/v1/messages/count_tokens") {
		claudeError(w, 404, "Not found")
		return
	}
	body, err := readObject(r.Body, 256<<20)
	if err != nil {
		claudeError(w, 400, "Invalid JSON: "+err.Error())
		return
	}
	if r.URL.Path == "/v1/messages/count_tokens" {
		respond(w, 200, object{"input_tokens": estimateTokens(body)})
		return
	}
	request := anthropicToOpenAI(body)
	log.Printf("request=%s model=%q stream=%t", requestLogID(r.Context()), str(body["model"]), body["stream"] == true)
	raw, err := a.discover(r.Context(), false)
	if err != nil {
		log.Printf("request=%s client=Claude model_discovery_error=%q", requestLogID(r.Context()), diagnosticError(err))
		request["model"] = defaultClaude
	} else {
		request["model"] = mapClaude(str(request["model"]), raw)
	}
	model := str(request["model"])
	if effort := str(request["reasoning_effort"]); effort != "" {
		a.mu.Lock()
		a.lastEffort = object{"source": "output_config.effort", "requested": nested(body, "output_config", "effort"), "effort": effort}
		a.mu.Unlock()
	}
	response, err := a.copilot(r.Context(), "POST", "/chat/completions", jsonBytes(request))
	if err != nil {
		claudeError(w, 502, err.Error())
		return
	}
	if response.StatusCode != 200 {
		data, e := readResponse(response, 16<<20)
		if e != nil {
			claudeError(w, 502, e.Error())
			return
		}
		var value object
		_ = json.Unmarshal(data, &value)
		message := str(nested(value, "error", "message"))
		if message == "" {
			message = str(value["message"])
		}
		if message == "" {
			message = string(data)
		}
		claudeError(w, response.StatusCode, message)
		return
	}
	if body["stream"] == true {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)
		err = translateStream(response.Body, w, model, estimateTokens(body))
		response.Body.Close()
		outcome := "completed"
		if err != nil {
			outcome = "error"
		}
		if r.Context().Err() != nil {
			outcome = "canceled"
		}
		log.Printf("request=%s client=Claude stream_end outcome=%s error=%q context_error=%q", requestLogID(r.Context()), outcome, diagnosticError(err), diagnosticError(r.Context().Err()))
		if err != nil {
			log.Printf("request=%s client=Claude upstream_stream_error=%q", requestLogID(r.Context()), diagnosticError(err))
		}
		return
	}
	data, err := readResponse(response, 64<<20)
	if err != nil {
		claudeError(w, 502, err.Error())
		return
	}
	var value object
	if json.Unmarshal(data, &value) != nil || value == nil {
		claudeError(w, 502, "Bad upstream JSON")
		return
	}
	respond(w, 200, anthropicResponse(value, model))
}

func timeISO() string { return time.Now().UTC().Format(time.RFC3339Nano) }
