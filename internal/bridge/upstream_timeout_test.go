package bridge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUpstreamSilenceIsBounded(t *testing.T) {
	for _, phase := range []string{"headers", "first-byte", "mid-stream"} {
		t.Run(phase, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if phase != "headers" {
					if phase == "mid-stream" {
						_, _ = io.WriteString(w, "first chunk")
					}
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			transport := &idleTransport{base: &http.Transport{ResponseHeaderTimeout: 50 * time.Millisecond}, timeout: 50 * time.Millisecond}
			defer transport.CloseIdleConnections()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			r, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			response, err := (&http.Client{Transport: transport}).Do(r)
			if phase != "headers" {
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				data, readErr := io.ReadAll(response.Body)
				err = readErr
				if phase == "mid-stream" && string(data) != "first chunk" {
					t.Fatal("received data was lost", string(data))
				}
			}
			var timeout interface{ Timeout() bool }
			if !errors.As(err, &timeout) || !timeout.Timeout() || ctx.Err() != nil {
				t.Fatal("silence did not time out independently of the caller", err, ctx.Err())
			}
		})
	}
}

func TestUpstreamIdleTimeoutResetsOnProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 20; i++ {
			_, _ = io.WriteString(w, "x")
			w.(http.Flusher).Flush()
			select {
			case <-time.After(20 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
	}))
	defer server.Close()
	transport := &idleTransport{base: &http.Transport{}, timeout: 200 * time.Millisecond}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport, Timeout: 3 * time.Second}).Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || string(data) != strings.Repeat("x", 20) {
		t.Fatal("active stream was cut off by a total timeout", string(data), err)
	}
}

func TestUpstreamCancellationIsPreserved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	transport := &idleTransport{base: &http.Transport{}, timeout: time.Second}
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	response, err := (&http.Client{Transport: transport}).Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	cancel()
	_, err = io.ReadAll(response.Body)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("caller cancellation was misreported as an upstream timeout", err)
	}
}
