package bridge

/*
import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type sharedConfig struct {
	Enabled bool   `json:"enabled"`
	Listen  string `json:"listen"`
	Token   string `json:"token"`
}

type remoteCatalog struct {
	Protocol int      `json:"protocol"`
	Name     string   `json:"name"`
	Models   []object `json:"models"`
	Codex    bool     `json:"codexEnabled"`
	Claude   bool     `json:"claudeEnabled"`
	Copilot  bool     `json:"copilotEnabled"`
}

func validAccessToken(token string) bool {
	if len(token) < 16 || len(token) > 8192 {
		return false
	}
	for _, c := range token {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}

func validateShared(config sharedConfig) error {
	host, port, err := net.SplitHostPort(config.Listen)
	number, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || net.ParseIP(host) == nil || number < 1 || number > 65535 {
		return errors.New("Server listen address must be an IP address and a nonzero port")
	}
	if config.Enabled && !validAccessToken(config.Token) {
		return errors.New("Server access token must contain 16 to 8192 printable ASCII characters without spaces")
	}
	return nil
}

func remoteURL(address string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(address), "/"))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" ||
		strings.Contains(u.Path, "..") || strings.ContainsAny(u.Path, "\\\x00") {
		return nil, errors.New("enter a Server HTTP(S) URL without credentials, query, or fragment")
	}
	// Plain HTTP is restricted to literal LAN addresses, so DNS changes cannot leak a token.
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if ip == nil || (!ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()) {
			return nil, errors.New("HTTP requires a private or loopback IP address; use HTTPS for hostnames and public servers")
		}
	}
	return u, nil
}

func (a *App) configureShared(body object) (any, error) {
	next := a.shared
	for key, value := range body {
		switch key {
		case "enabled":
			var ok bool
			next.Enabled, ok = value.(bool)
			if !ok {
				return nil, errors.New("enabled must be a boolean")
			}
		case "listen", "token":
			v, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be a string", key)
			}
			if key == "listen" {
				next.Listen = strings.TrimSpace(v)
			} else if v != "" {
				next.Token = v
			}
		default:
			return nil, fmt.Errorf("unknown Server setting: %s", key)
		}
	}
	if next.Token == "" {
		next.Token = rand.Text()
	}
	if err := validateShared(next); err != nil {
		return nil, err
	}
	if next.Enabled && a.snapshot().Token == "" {
		return nil, errors.New("connect to GitHub on this Kobashi before enabling Server")
	}
	previous := a.shared
	if next.Enabled && a.sharedServer != nil && next.Listen != previous.Listen {
		return nil, errors.New("disable Server before changing its listen address")
	}
	a.mu.Lock()
	a.shared = next
	a.mu.Unlock()
	if next.Enabled && a.sharedServer == nil {
		if err := a.startShared(); err != nil {
			a.mu.Lock()
			a.shared = previous
			a.mu.Unlock()
			return nil, err
		}
	}
	if err := writeJSON(a.path("server.json"), next); err != nil {
		if !previous.Enabled {
			a.stopShared()
		}
		a.mu.Lock()
		a.shared = previous
		a.mu.Unlock()
		return nil, err
	}
	if !next.Enabled {
		a.stopShared()
	}
	return next, nil
}

func (a *App) startShared() error {
	if a.sharedServer != nil {
		return nil
	}
	if a.snapshot().Token == "" || a.snapshot().RemoteURL != "" {
		return errors.New("Server requires a local GitHub connection")
	}
	listener, err := net.Listen("tcp", a.shared.Listen)
	if err != nil {
		return fmt.Errorf("start Server: %w", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(a.serveShared), ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout: 90 * time.Second, BaseContext: func(net.Listener) context.Context { return a.ctx }}
	a.mu.Lock()
	a.sharedServer = server
	a.mu.Unlock()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.report(fmt.Errorf("Server: %w", err))
		}
	}()
	service, err := advertiseLAN(a.shared.Listen)
	if err != nil {
		a.report(fmt.Errorf("Server discovery unavailable; use a manual URL: %w", err))
	} else {
		a.sharedDiscovery = service
	}
	return nil
}

func (a *App) stopShared() {
	if a.sharedDiscovery != nil {
		a.sharedDiscovery.Close()
		a.sharedDiscovery = nil
	}
	a.mu.Lock()
	server := a.sharedServer
	a.sharedServer = nil
	a.mu.Unlock()
	if server != nil {
		a.report(server.Close())
	}
}

func (a *App) serveShared(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	config := a.shared
	a.mu.RUnlock()
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !config.Enabled || !validAccessToken(config.Token) ||
		subtle.ConstantTimeCompare([]byte(token), []byte(config.Token)) != 1 {
		respond(w, 401, object{"error": "Valid Kobashi Server token required"})
		return
	}
	if r.Header.Get("Origin") != "" {
		respond(w, 403, object{"error": "Browser origins are not allowed"})
		return
	}
	if r.Method == "GET" && r.URL.Path == "/kobashi/v1/catalog" {
		state := a.snapshot()
		if state.Codex {
			if err := a.models.ensureSession(r.Context()); err != nil {
				respond(w, 502, object{"error": err.Error()})
				return
			}
			a.models.wg.Wait()
		}
		respond(w, 200, remoteCatalog{1, "Kobashi", a.models.visible(), state.Codex, state.Claude, state.Copilot})
		return
	}
	protocol, path, found := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	path = "/" + path
	allowed := r.Method == "GET" && path == "/v1/models" ||
		r.Method == "POST" && (((protocol == "codex" || protocol == "copilot") && contains([]string{"/v1/responses", "/v1/responses/compact", "/v1/chat/completions"}, path)) ||
			(protocol == "claude" && contains([]string{"/v1/messages", "/v1/messages/count_tokens"}, path)))
	if !found || !allowed || (protocol != "codex" && protocol != "claude" && protocol != "copilot") {
		respond(w, 404, object{"error": "Inference endpoint not found"})
		return
	}
	r = r.Clone(r.Context())
	r.URL.Path, r.URL.RawPath = path, ""
	r.RequestURI = r.URL.RequestURI()
	if protocol == "copilot" {
		a.openAI(w, r, true)
	} else if protocol == "codex" {
		a.codex(w, r)
	} else {
		a.claude(w, r)
	}
}

func (a *App) fetchRemoteCatalog(ctx context.Context, state session) (remoteCatalog, error) {
	var catalog remoteCatalog
	base, err := remoteURL(state.RemoteURL)
	if err != nil {
		return catalog, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	base.Path += "/kobashi/v1/catalog"
	req, err := http.NewRequestWithContext(ctx, "GET", base.String(), nil)
	if err != nil {
		return catalog, err
	}
	req.Header.Set("Authorization", "Bearer "+state.RemoteToken)
	response, err := a.remoteClient.Do(req)
	if err != nil {
		return catalog, fmt.Errorf("connect to Kobashi Server: %w", err)
	}
	data, err := readResponse(response, 16<<20)
	if err != nil {
		return catalog, err
	}
	if response.StatusCode != 200 {
		return catalog, fmt.Errorf("Kobashi Server returned HTTP %d; check its address, token, and enabled bridges", response.StatusCode)
	}
	if json.Unmarshal(data, &catalog) != nil || catalog.Protocol != 1 || catalog.Models == nil {
		return catalog, errors.New("Server returned an invalid Kobashi model catalog")
	}
	return catalog, nil
}

func (a *App) refreshRemoteCatalog(ctx context.Context, state session) error {
	catalog, err := a.fetchRemoteCatalog(ctx, state)
	if err != nil {
		return err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.state.RemoteURL != state.RemoteURL || a.state.RemoteToken != state.RemoteToken {
		return errors.New("remote connection changed during model discovery")
	}
	result := buildCatalog(nil, catalog.Models)
	if len(arr(result["models"])) == 0 {
		sentinel := fallbackModel("kobashi-no-available-models", "No models available through Kobashi Server", 1)
		sentinel["visibility"], sentinel["supported_in_api"] = "hide", false
		result["models"] = []any{sentinel}
	}
	return writeJSON(a.path("codex-model-catalog.json"), result)
}

func (a *App) connectRemote(ctx context.Context, address, token string) error {
	if a.snapshot().active() {
		return errors.New("disconnect the current source before connecting to a Server")
	}
	if a.shared.Enabled {
		return errors.New("disable Server before switching to Client")
	}
	base, err := remoteURL(address)
	if err != nil {
		return err
	}
	if !validAccessToken(token) {
		return errors.New("enter the Server access token (at least 16 characters)")
	}
	next := session{RemoteURL: base.String(), RemoteToken: token, Method: "remote", Username: base.Host}
	catalog, err := a.fetchRemoteCatalog(ctx, next)
	if err != nil {
		return err
	}
	if !catalog.Codex && !catalog.Claude && !catalog.Copilot {
		return errors.New("Server has no enabled bridges")
	}
	next.Codex, next.Claude = catalog.Codex, catalog.Claude
	next.Copilot = catalog.Copilot
	a.mu.Lock()
	a.state = next
	a.mu.Unlock()
	if err = a.saveSession(); err == nil && next.Codex {
		err = a.injectCodex()
	}
	if err == nil && next.Claude {
		err = a.injectClaude()
	}
	if err != nil {
		return errors.Join(err, a.disconnect())
	}
	return nil
}

func (a *App) forwardRemote(w http.ResponseWriter, r *http.Request, state session, protocol string, enabled bool) {
	if !enabled || !state.active() {
		respond(w, 401, object{"error": "Remote bridge not active"})
		return
	}
	base, err := remoteURL(state.RemoteURL)
	if err != nil {
		respond(w, 502, object{"error": err.Error()})
		return
	}
	base.Path += "/" + protocol
	proxy := &httputil.ReverseProxy{
		Transport: a.remoteClient.Transport, FlushInterval: -1,
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(base)
			p.Out.Header = http.Header{}
			for _, key := range []string{"Content-Type", "Accept", "Content-Encoding", "Anthropic-Version", "Anthropic-Beta", "OpenAI-Beta"} {
				if values := p.In.Header.Values(key); len(values) > 0 {
					p.Out.Header[key] = values
				}
			}
			p.Out.Header.Set("Authorization", "Bearer "+state.RemoteToken)
			p.Out.Header.Set("User-Agent", "Kobashi-Client/"+a.options.Version)
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			log.Printf("Kobashi Server request: %v", err)
			status := http.StatusBadGateway
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				status = http.StatusRequestEntityTooLarge
			}
			respond(w, status, object{"error": "Kobashi Server request failed: " + err.Error()})
		},
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<20)
	proxy.ServeHTTP(w, r)
}
*/
