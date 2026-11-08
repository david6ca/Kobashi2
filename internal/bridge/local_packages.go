package bridge

import (
	"context"
	"errors"
	"html"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var localPackageName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*/$`)
var localPackageFeedHost = regexp.MustCompile(`^(ms-feed-[0-9]+\.pkgs\.visualstudio\.com|[a-z0-9-]+\.vsblob\.vsassets\.io)$`)
var localPackageLinks = regexp.MustCompile(`(?i)\bhref\s*=\s*(?:"([^"]*)"|'([^']*)')`)
var localPackageMetadata = regexp.MustCompile(`(?i)\s+data-(core|dist-info)-metadata(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+))?`)

func approvedLocalPackageURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && (u.Port() == "" || u.Port() == "443") &&
		(u.Hostname() == "packagefeedproxy.microsoft.io" || localPackageFeedHost.MatchString(u.Hostname()))
}

func localPackageHTML(data []byte) ([]byte, error) {
	for _, match := range localPackageLinks.FindAllSubmatch(data, -1) {
		address := string(match[1])
		if address == "" {
			address = string(match[2])
		}
		u, err := url.Parse(html.UnescapeString(address))
		if err != nil || !u.IsAbs() || !approvedLocalPackageURL(u) {
			return nil, errors.New("Microsoft package feed returned an unsupported download source")
		}
	}
	// The Microsoft feed advertises PEP 658 sidecars that can return 404.
	// Retain wheel URLs and hashes; let uv read metadata from the actual wheel.
	return localPackageMetadata.ReplaceAll(data, nil), nil
}

func (l *localManager) packageIndex(ctx context.Context) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	client := *l.app.client
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !approvedLocalPackageURL(r.URL) {
			return errors.New("Microsoft package feed redirected to an unsupported source")
		}
		return nil
	}
	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       30 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name, ok := strings.CutPrefix(r.URL.Path, "/simple/")
			if r.Method != http.MethodGet || !ok || len(name) > 128 || !localPackageName.MatchString(name) || r.URL.RawQuery != "" {
				http.NotFound(w, r)
				return
			}
			fail := func(err error) {
				log.Printf("Local Microsoft package index failed: %s", diagnosticError(err))
				http.Error(w, "Microsoft package feed is unavailable; check setup.log and app diagnostics", http.StatusBadGateway)
			}
			upstream, err := http.NewRequestWithContext(r.Context(), http.MethodGet, localPackageIndex+name, nil)
			if err != nil {
				fail(err)
				return
			}
			upstream.Header.Set("Accept", "text/html")
			response, err := client.Do(upstream)
			if err != nil {
				fail(errors.New("Could not reach the Microsoft package feed"))
				return
			}
			code := response.StatusCode
			data, err := readResponse(response, 16<<20)
			if err != nil {
				fail(err)
				return
			}
			if code != http.StatusOK {
				http.Error(w, "Microsoft package feed returned "+http.StatusText(code), code)
				return
			}
			data, err = localPackageHTML(data)
			if err != nil {
				fail(err)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if _, err = w.Write(data); err != nil {
				log.Printf("Local package index response failed: %s", diagnosticError(err))
			}
		}),
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Local Microsoft package index server failed: %s", diagnosticError(err))
		}
	}()
	return "http://" + listener.Addr().String() + "/simple/", func() { server.Close() }, nil
}
