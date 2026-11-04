package bridge

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type diagnosticBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *diagnosticBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(data)
}
func (b *diagnosticBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}
func captureDiagnostics(t *testing.T) *diagnosticBuffer {
	t.Helper()
	previous := log.Writer()
	buffer := &diagnosticBuffer{}
	log.SetOutput(buffer)
	t.Cleanup(func() { log.SetOutput(previous) })
	return buffer
}

func TestProxyDiagnosticsStreamEndings(t *testing.T) {
	for _, client := range []string{"Codex", "Copilot"} {
		for _, fixture := range []struct {
			name, path, source, outcome string
			readError                   bool
		}{
			{"completed", "/v1/responses", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n", "response.completed", false},
			{"failed", "/v1/responses", "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"upstream failed\"}}}\n\n", "response.failed", false},
			{"incomplete", "/v1/responses", "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\"}}\n\n", "response.incomplete", false},
			{"early-eof", "/v1/responses", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"private-output\"}\n\n", "eof_without_terminal", false},
			{"read-error", "/v1/responses", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"private-output\"}\n\n", "io_error", true},
			{"chat-completed", "/v1/chat/completions", "data: {\"choices\":[{\"delta\":{\"content\":\"private-output\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "chat.finish", false},
			{"chat-early-eof", "/v1/chat/completions", "data: {\"choices\":[{\"delta\":{\"content\":\"private-output\"}}]}\n\n", "eof_without_terminal", false},
		} {
			t.Run(client+"/"+fixture.name, func(t *testing.T) {
				logs := captureDiagnostics(t)
				calls := 0
				a := testApp(t, func(r *http.Request) (*http.Response, error) {
					calls++
					result := response(200, nil)
					result.Header.Set("Content-Type", "text/event-stream")
					result.Header.Set("X-Request-Id", "test-upstream-id")
					var source io.Reader = strings.NewReader(fixture.source)
					if fixture.readError {
						source = io.MultiReader(source, brokenStreamReader{})
					}
					result.Body = io.NopCloser(source)
					return result, nil
				})
				a.state = session{Token: "private-account-token", Codex: true, Copilot: true}
				a.bearer, a.expiry = "private-upstream-token", float64(time.Now().Add(time.Hour).Unix())
				a.models.sessionStarted = true
				r := httptest.NewRequest("POST", fixture.path+"?private-query", strings.NewReader(`{"model":"gpt-test","stream":true,"input":"private-input"}`))
				w := httptest.NewRecorder()
				if client == "Copilot" {
					r.Header.Set("Authorization", "Bearer PROXY_MANAGED")
					a.copilotApp(w, r)
				} else {
					a.codex(w, r)
				}
				got := w.Body.String()
				if fixture.outcome == "eof_without_terminal" || fixture.readError {
					if !strings.Contains(w.Body.String(), `"code":"upstream_stream_interrupted"`) || strings.Contains(w.Body.String(), `"type":"response.completed"`) {
						t.Fatal("interrupted stream did not deliver an explicit error", w.Body.String())
					}
					got = strings.Split(w.Body.String(), "\n\nevent: ")[0]
				}
				if calls != 1 || w.Code != 200 || got != fixture.source || !w.Flushed {
					t.Fatal("diagnostics changed stream, flush, or replay behavior", calls, w.Code, w.Body.String())
				}
				text := logs.String()
				for _, expected := range []string{"client=" + client + " begin", `model="gpt-test" stream=true`, "headers status=200", `request_id="test-upstream-id"`, "body_end bytes=", "stream_end outcome=" + fixture.outcome, "client=" + client + " end status=200"} {
					if !strings.Contains(text, expected) {
						t.Fatalf("missing %q:\n%s", expected, text)
					}
				}
				ids := regexp.MustCompile(`request=([A-Z2-7]+)`).FindAllStringSubmatch(text, -1)
				if len(ids) < 6 {
					t.Fatal("missing request correlation", text)
				}
				for _, id := range ids {
					if id[1] != ids[0][1] {
						t.Fatal("request correlation changed", text)
					}
				}
				for _, secret := range []string{"private-account-token", "private-upstream-token", "private-input", "private-output", "private-query", "PROXY_MANAGED"} {
					if strings.Contains(text, secret) {
						t.Fatalf("payload or credential %q was logged", secret)
					}
				}
				if fixture.readError && !strings.Contains(text, `read_error="unexpected EOF"`) {
					t.Fatal("lost upstream read failure", text)
				}
			})
		}
	}
}

func TestProxyDiagnosticsTransportAndHTTPFailures(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			logs := captureDiagnostics(t)
			a := testApp(t, func(*http.Request) (*http.Response, error) {
				result := response(status, object{"error": object{"code": "test_error", "message": "test failure"}})
				result.Header.Set("Retry-After", "30")
				return result, nil
			})
			a.state = session{Token: "account", Codex: true}
			a.bearer, a.expiry = "bearer", float64(time.Now().Add(time.Hour).Unix())
			a.models.sessionStarted = true
			if status == 401 {
				a.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if r.URL.Host == "api.github.com" {
						return response(200, object{"token": "new-bearer", "expires_at": time.Now().Add(time.Hour).Unix()}), nil
					}
					return response(401, object{"error": object{"message": "Expired"}}), nil
				})
			}
			w := httptest.NewRecorder()
			a.codex(w, httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-test"}`)))
			if w.Code != status || !strings.Contains(logs.String(), "body_end bytes=") {
				t.Fatal("lost HTTP error diagnostics", w.Code, logs.String())
			}
			if status == 401 && (!strings.Contains(logs.String(), "retry reason=authentication") || !strings.Contains(logs.String(), "retry_exhausted reason=authentication")) {
				t.Fatal("lost authentication retry diagnostics", logs.String())
			}
			if status == 429 && !strings.Contains(logs.String(), `retry_after="30"`) {
				t.Fatal("lost throttling diagnostics", logs.String())
			}
		})
	}
	t.Run("transport-canceled", func(t *testing.T) {
		logs := captureDiagnostics(t)
		a := testApp(t, func(r *http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: "Post", URL: "https://user:private-password@example.invalid/responses?private-query", Err: context.Canceled}
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := a.request(ctx, "POST", "https://user:private-password@example.invalid/responses?private-query", []byte("private-input"), headers("private-token"))
		if !errors.Is(err, context.Canceled) {
			t.Fatal("transport error changed", err)
		}
		text := logs.String()
		if !strings.Contains(text, `transport_error="context canceled"`) || !strings.Contains(text, `context_error="context canceled"`) {
			t.Fatal("missing cancellation diagnostics", text)
		}
		for _, secret := range []string{"private-password", "private-query", "private-token", "private-input"} {
			if strings.Contains(text, secret) {
				t.Fatal("transport diagnostics exposed private data", text)
			}
		}
	})
}

type failedResponseWriter struct{ header http.Header }

func (w *failedResponseWriter) Header() http.Header { return w.header }
func (*failedResponseWriter) WriteHeader(int)       {}
func (*failedResponseWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}
func (*failedResponseWriter) FlushError() error { return io.ErrClosedPipe }

func TestProxyDiagnosticsDownstreamFailures(t *testing.T) {
	logs := captureDiagnostics(t)
	base := &failedResponseWriter{header: http.Header{}}
	w, r, done := logProxyRequest(base, httptest.NewRequest("POST", "/v1/responses", nil), "Codex")
	n, err := w.Write([]byte("private-output"))
	if n != 0 || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("write error changed", n, err)
	}
	w.(http.Flusher).Flush()
	if !errors.Is(http.NewResponseController(w).SetWriteDeadline(time.Now()), http.ErrNotSupported) {
		t.Fatal("response controller could not unwrap the diagnostic writer")
	}
	done()
	text := logs.String()
	if requestLogID(r.Context()) == "background" || !strings.Contains(text, `write_error="io: read/write on closed pipe"`) || strings.Contains(text, "private-output") {
		t.Fatal("lost downstream failure or leaked payload", text)
	}
}

func TestClaudeDiagnosticsStreamEndings(t *testing.T) {
	for _, finished := range []bool{false, true} {
		t.Run(map[bool]string{false: "early-eof", true: "completed"}[finished], func(t *testing.T) {
			logs := captureDiagnostics(t)
			source := "data: {\"choices\":[{\"delta\":{\"content\":\"private-output\"}}]}\n\n"
			if finished {
				source += "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
			}
			calls := 0
			a := testApp(t, func(*http.Request) (*http.Response, error) {
				calls++
				result := response(200, nil)
				result.Header.Set("Content-Type", "text/event-stream")
				result.Body = io.NopCloser(strings.NewReader(source))
				return result, nil
			})
			a.state = session{Token: "private-token", Claude: true}
			a.bearer, a.expiry = "private-bearer", float64(time.Now().Add(time.Hour).Unix())
			a.rawModels, a.rawAt = []object{}, time.Now()
			w := httptest.NewRecorder()
			a.claude(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"claude-test","stream":true,"messages":[{"role":"user","content":"private-input"}]}`)))
			if calls != 1 || w.Code != 200 || strings.Contains(w.Body.String(), "message_stop") != finished {
				t.Fatal("Claude stream completion or request count changed", calls, w.Code, w.Body.String())
			}
			text := logs.String()
			if !strings.Contains(text, "client=Claude stream_end outcome=") || !strings.Contains(text, "client=Claude end status=200") {
				t.Fatal("missing Claude diagnostics", text)
			}
			if !finished && !strings.Contains(text, "upstream stream ended before completion") {
				t.Fatal("missing Claude early EOF", text)
			}
			for _, secret := range []string{"private-token", "private-bearer", "private-input", "private-output"} {
				if strings.Contains(text, secret) {
					t.Fatal("Claude diagnostics exposed private data", text)
				}
			}
		})
	}
}

func TestUpstreamDiagnosticsEarlyCloseAndConnectionReuse(t *testing.T) {
	logs := captureDiagnostics(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Github-Request-Id", "test-github-id")
		_, _ = io.WriteString(w, "private-body")
	}))
	defer server.Close()
	a := testApp(t, nil)
	a.client = &http.Client{Transport: &idleTransport{base: &http.Transport{}, timeout: time.Second}}
	defer a.client.CloseIdleConnections()
	for range 2 {
		result, err := a.request(context.Background(), "GET", server.URL+"/models?private-query", nil, headers("private-token"))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, result.Body)
		if err := result.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
	result, err := a.request(context.Background(), "GET", server.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Body.Close(); err != nil {
		t.Fatal(err)
	}
	text := logs.String()
	for _, expected := range []string{"connection remote=", "reused=true", `github_request_id="test-github-id"`, "eof=true", "eof=false"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "private-") {
		t.Fatal("upstream diagnostics leaked payload, token, or query", text)
	}
}

func TestProxySignalsUpstreamTimeoutWithoutReplaying(t *testing.T) {
	logs := captureDiagnostics(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	transport := &idleTransport{base: &http.Transport{}, timeout: 50 * time.Millisecond}
	defer transport.CloseIdleConnections()
	calls := 0
	a := testApp(t, func(r *http.Request) (*http.Response, error) {
		calls++
		upstream := r.Clone(r.Context())
		upstream.URL, _ = url.Parse(server.URL)
		return transport.RoundTrip(upstream)
	})
	a.state = session{Token: "account", Codex: true}
	a.bearer, a.expiry = "bearer", float64(time.Now().Add(time.Hour).Unix())
	a.models.sessionStarted = true
	w := httptest.NewRecorder()
	a.codex(w, httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-test","stream":true}`)))
	if calls != 1 || !strings.Contains(w.Body.String(), `"code":"upstream_timeout"`) || strings.Contains(w.Body.String(), "response.completed") {
		t.Fatal("timeout was hidden, replayed, or reported as success", calls, w.Body.String())
	}
	if !strings.Contains(logs.String(), `read_error="upstream response idle timeout"`) || !strings.Contains(logs.String(), `context_error=""`) {
		t.Fatal("upstream timeout was misreported as a caller cancellation", logs.String())
	}
}
