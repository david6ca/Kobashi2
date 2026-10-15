package bridge

/*
import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testServerToken = "kobashi-test-access-token"

func remoteFixture(t *testing.T, upstream roundTripFunc) (*App, *httptest.Server) {
	t.Helper()
	a := testApp(t, upstream)
	a.state = session{Token: "github-server-only", Username: "server-user", Codex: true, Claude: true, Method: "device"}
	a.bearer, a.expiry = "copilot-server-only", float64(time.Now().Add(time.Hour).Unix())
	a.shared = sharedConfig{true, "127.0.0.1:18924", testServerToken}
	a.models.sessionStarted = true
	model := compatibleModel("gpt-remote")
	a.models.latest["gpt-remote"] = modelEntry{Model: model}
	a.models.records["gpt-remote"] = &modelRecord{ID: "gpt-remote", State: "verified", Constraints: constraints{}}
	server := httptest.NewServer(http.HandlerFunc(a.serveShared))
	t.Cleanup(server.Close)
	return a, server
}

func TestSharedServerAuthenticationAndEndpointIsolation(t *testing.T) {
	a, server := remoteFixture(t, func(*http.Request) (*http.Response, error) {
		t.Error("blocked request reached Copilot")
		return nil, errors.New("unexpected upstream call")
	})
	for _, test := range []struct {
		path, method, token, origin string
		code                        int
	}{
		{"/kobashi/v1/catalog", "GET", "", "", 401},
		{"/kobashi/v1/catalog", "GET", a.ControlToken(), "", 401},
		{"/kobashi/v1/catalog", "GET", "wrong-token", "", 401},
		{"/kobashi/v1/catalog", "GET", testServerToken, "https://example.invalid", 403},
		{"/api/status", "GET", testServerToken, "", 404},
		{"/api/quit", "POST", testServerToken, "", 404},
		{"/codex/api/disconnect", "POST", testServerToken, "", 404},
		{"/codex/v1/models", "DELETE", testServerToken, "", 404},
		{"/codex/v1/arbitrary-upstream", "POST", testServerToken, "", 404},
		{"/kobashi/v1/catalog", "GET", testServerToken, "", 200},
	} {
		req, _ := http.NewRequest(test.method, server.URL+test.path, nil)
		req.Header.Set("Authorization", "Bearer "+test.token)
		req.Header.Set("Origin", test.origin)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, err := readResponse(response, 1<<20)
		if err != nil || response.StatusCode != test.code {
			t.Fatalf("%s %s: %d %s %v", test.method, test.path, response.StatusCode, data, err)
		}
		for _, secret := range []string{a.state.Token, a.bearer, a.ControlToken(), testServerToken} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("Server disclosed a credential on %s", test.path)
			}
		}
	}
}

func TestRemoteClientWithoutGitHubPreservesInferenceAndConfiguration(t *testing.T) {
	input := `{"model":"gpt-remote","input":[{"type":"agent_message","content":[{"text":"encrypted-payload"}]}],"stream":true}`
	stream := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n"
	var calls atomic.Int32
	_, server := remoteFixture(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Host != "api.githubcopilot.com" || r.Header.Get("Authorization") != "Bearer copilot-server-only" {
			t.Error("Server did not use its own upstream credentials")
		}
		if r.URL.Path == "/v1/responses" {
			data, _ := io.ReadAll(r.Body)
			if string(data) != input {
				t.Error("Client changed inference input or encrypted messages")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
		}
		if r.URL.Path == "/models" {
			return response(200, object{"data": []any{object{"id": "claude-sonnet-4.6"}}}), nil
		}
		if r.URL.Path == "/chat/completions" {
			return response(200, object{"id": "chat-remote", "choices": []any{object{"message": object{"role": "assistant", "content": "hello"}, "finish_reason": "stop"}}}), nil
		}
		return nil, errors.New("unexpected upstream endpoint: " + r.URL.Path)
	})
	client := testApp(t, func(*http.Request) (*http.Response, error) {
		t.Error("Client contacted GitHub or Copilot directly")
		return nil, errors.New("Client must use its Server")
	})
	configPath := filepath.Join(client.options.CodexHome, "config.toml")
	authPath := filepath.Join(client.options.CodexHome, "auth.json")
	config := "notify = [\"keep-me\"]\n"
	auth := []byte(`{"tokens":{"access_token":"chatgpt-user-token","id_token":"chatgpt-user-id"}}`)
	if err := writeAtomic(configPath, []byte(config)); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(authPath, auth); err != nil {
		t.Fatal(err)
	}
	result := controlCall(client, "/api/remote-connect", object{"url": server.URL, "token": testServerToken})
	if result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	state := client.snapshot()
	if state.Token != "" || state.RemoteURL != server.URL || !state.Codex || !state.Claude {
		t.Fatal("Client requires local GitHub credentials or lost remote state")
	}
	status := controlCall(client, "/api/status", nil)
	if !strings.Contains(status.Body.String(), `"connected":true`) || strings.Contains(status.Body.String(), testServerToken) {
		t.Fatal("invalid Client status", status.Body.String())
	}
	catalog, err := os.ReadFile(client.path("codex-model-catalog.json"))
	if err != nil || !strings.Contains(string(catalog), `"slug": "gpt-remote"`) {
		t.Fatal("Server models missing from Client catalog", err)
	}
	codex := httptest.NewServer(http.HandlerFunc(client.codex))
	defer codex.Close()
	req, _ := http.NewRequest("POST", codex.URL+"/v1/responses", strings.NewReader(input))
	req.Header.Set("Authorization", "Bearer chatgpt-user-token")
	req.Header.Set("Cookie", "private-user-cookie")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := readResponse(response, 1<<20)
	if err != nil || response.StatusCode != 200 || string(data) != stream {
		t.Fatalf("stream changed: %d %s %v", response.StatusCode, data, err)
	}
	claude := httptest.NewRecorder()
	client.claude(claude, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4.6","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`)))
	if claude.Code != 200 || !strings.Contains(claude.Body.String(), `"text":"hello"`) {
		t.Fatalf("Claude forwarding failed: %d %s", claude.Code, claude.Body.String())
	}
	models := httptest.NewRecorder()
	client.codex(models, httptest.NewRequest("GET", "/v1/models", nil))
	if models.Code != 200 || !strings.Contains(models.Body.String(), "gpt-remote") {
		t.Fatal("remote model list missing", models.Body.String())
	}
	if calls.Load() != 3 {
		t.Fatal("Client model preparation made extra inference requests", calls.Load())
	}
	reloaded, err := New(client.options)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.initialize()
	if reloaded.snapshot().RemoteURL != server.URL || reloaded.snapshot().Token != "" {
		t.Fatal("remote connection was not restored")
	}
	if err := reloaded.Stop(); err != nil {
		t.Fatal(err)
	}
	if result := controlCall(client, "/api/disconnect", object{}); result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	restored, _ := os.ReadFile(configPath)
	restoredAuth, _ := os.ReadFile(authPath)
	if !strings.Contains(string(restored), config) || strings.Contains(string(restored), "model_provider") || string(restoredAuth) != string(auth) {
		t.Fatal("disconnect damaged user configuration or ChatGPT login")
	}
}

func TestRemoteForwardingHeadersErrorsStreamingAndCancellation(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testServerToken || r.Header.Get("Cookie") != "" ||
			r.Header.Get("X-Api-Key") != "" || r.Header.Get("Openai-Organization") != "" || r.Header.Get("X-Forwarded-For") != "" {
			t.Error("Client leaked local credentials or forwarding headers")
		}
		if r.URL.Path != "/prefix/codex/v1/responses" {
			t.Error("base URL path or protocol was lost", r.URL.Path)
		}
		if r.URL.Query().Get("mode") == "error" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(429)
			io.WriteString(w, `{"error":{"code":"rate_limit","message":"original error"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer server.Close()
	client := testApp(t, nil)
	client.state = session{RemoteURL: server.URL + "/prefix", RemoteToken: testServerToken, Codex: true}
	forward := httptest.NewServer(http.HandlerFunc(client.codex))
	defer forward.Close()
	req, _ := http.NewRequest("POST", forward.URL+"/v1/responses?mode=error", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer private-chatgpt-token")
	req.Header.Set("X-Api-Key", "private-api-key")
	req.Header.Set("Cookie", "private-cookie")
	req.Header.Set("Openai-Organization", "private-org")
	req.Header.Set("X-Forwarded-For", "private-client")
	result, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := readResponse(result, 1<<20)
	if err != nil || result.StatusCode != 429 || string(data) != `{"error":{"code":"rate_limit","message":"original error"}}` {
		t.Fatal("upstream error was hidden or changed", result.StatusCode, string(data), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ = http.NewRequestWithContext(ctx, "POST", forward.URL+"/v1/responses", strings.NewReader("{}"))
	result, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("stream was buffered")
	}
	buffer := make([]byte, len("data: first\n\n"))
	if _, err := io.ReadFull(result.Body, buffer); err != nil || string(buffer) != "data: first\n\n" {
		t.Fatal("first event was not flushed", err)
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("Client cancellation did not reach Server")
	}
}

func TestRemoteConnectionValidationAndFailurePreserveState(t *testing.T) {
	for _, address := range []string{"http://example.com", "http://8.8.8.8", "http://user:password@192.168.1.2", "ftp://192.168.1.2", "https://server/?token=secret", "https://server/#fragment", "https://server/../api", "https://server/?"} {
		if _, err := remoteURL(address); err == nil {
			t.Fatal("unsafe remote URL accepted", address)
		}
	}
	for _, address := range []string{"http://127.0.0.1:18924", "http://192.168.1.2:18924", "http://[::1]:18924", "https://server.example/kobashi"} {
		if _, err := remoteURL(address); err != nil {
			t.Fatal(address, err)
		}
	}
	client := testApp(t, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer server.Close()
	if result := controlCall(client, "/api/remote-connect", object{"url": server.URL, "token": testServerToken}); result.Code != 400 {
		t.Fatal("wrong token accepted", result.Body.String())
	}
	if client.snapshot().active() {
		t.Fatal("failed connection changed session")
	}
	client.state = session{Token: "existing-login", Username: "keep-user"}
	if err := client.connectRemote(context.Background(), server.URL, testServerToken); err == nil || client.snapshot().Token != "existing-login" {
		t.Fatal("remote connection replaced an existing account")
	}
}

func TestServerConfigurationPersistenceRotationAndShutdown(t *testing.T) {
	a := testApp(t, nil)
	port, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := port.Addr().String()
	port.Close()
	a.state.Token = "server-github-account"
	result := controlCall(a, "/api/server", object{"enabled": true, "listen": address})
	if result.Code != 200 || a.sharedServer == nil || !validAccessToken(a.shared.Token) {
		t.Fatal("Server did not start", result.Body.String())
	}
	var config sharedConfig
	if err := json.Unmarshal(result.Body.Bytes(), &config); err != nil {
		t.Fatal(err)
	}
	if config.Token == a.ControlToken() {
		t.Fatal("Server reused management token")
	}
	result = controlCall(a, "/api/server", object{"enabled": true, "token": testServerToken})
	if result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	req, _ := http.NewRequest("GET", "http://"+address+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+config.Token)
	denied, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	denied.Body.Close()
	if denied.StatusCode != 401 {
		t.Fatal("rotated token still authorized")
	}
	saved, err := New(a.options)
	if err != nil || saved.shared.Token != testServerToken || !saved.shared.Enabled {
		t.Fatal("Server settings were not persisted", err)
	}
	saved.Stop()
	if result = controlCall(a, "/api/server", object{"enabled": false}); result.Code != 200 || a.sharedServer != nil {
		t.Fatal("Server did not stop independently", result.Body.String())
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal("Server port was not released", err)
	}
	listener.Close()
}

func TestMDNSAnnouncementParsingAndMalformedPackets(t *testing.T) {
	packet := lanAnnouncement("test."+lanServiceName, "test.local.", net.IPv4(192, 168, 1, 10), 18924, 120)
	answers := lanAnswers(packet, net.IPv4(192, 168, 1, 10))
	if len(answers) != 1 || answers[0].URL != "http://192.168.1.10:18924" || answers[0].Name != "test" {
		t.Fatal("invalid mDNS discovery", answers)
	}
	answers = lanAnswers(packet, net.IPv4(192, 168, 1, 20))
	if len(answers) != 1 || answers[0].URL != "http://192.168.1.10:18924" {
		t.Fatal("mDNS ignored the Server's advertised listen address", answers)
	}
	if len(lanAnswers(packet, net.IPv4(8, 8, 8, 8))) != 0 {
		t.Fatal("public discovery source accepted")
	}
	for n := 0; n < len(packet); n++ {
		if len(lanAnswers(packet[:n], net.IPv4(192, 168, 1, 10))) != 0 {
			t.Fatal("truncated mDNS answer accepted", n)
		}
	}
	loop := []byte{0xc0, 0}
	if _, _, err := readDNSName(loop, 0); err == nil {
		t.Fatal("compression pointer loop accepted")
	}
	compressed := append(dnsName("test.local."), 0xc0, 0)
	if name, _, err := readDNSName(compressed, len(compressed)-2); err != nil || name != "test.local." {
		t.Fatal("DNS compression failed", name, err)
	}
	binary.BigEndian.PutUint16(packet[6:8], 65535)
	if len(lanAnswers(packet, net.IPv4(192, 168, 1, 10))) != 0 {
		t.Fatal("oversized record count accepted")
	}
}

func TestLANDiscoveryFindsRunningServer(t *testing.T) {
	if interfaces, err := lanInterfaces(); err != nil || len(interfaces) == 0 {
		t.Skip("no IPv4 multicast interface", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	defer listener.Close()
	service, err := advertiseLAN(net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
	if err != nil {
		t.Skip("mDNS unavailable in this environment", err)
	}
	defer service.Close()
	found, err := discoverLAN(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, server := range found["servers"].([]lanServer) {
		if strings.HasSuffix(server.URL, ":"+strconv.Itoa(port)) {
			return
		}
	}
	t.Fatal("the test Server was not discovered", found)
}
*/
