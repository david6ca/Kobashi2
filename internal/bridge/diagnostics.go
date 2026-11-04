package bridge

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sync"
	"time"
)

type requestLogKey struct{}

func requestLogID(ctx context.Context) string {
	id, _ := ctx.Value(requestLogKey{}).(string)
	if id == "" {
		return "background"
	}
	return id
}

func diagnosticError(err error) string {
	if err == nil {
		return ""
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		// URLs in transport errors can include credentials or query parameters.
		return diagnosticError(urlError.Err)
	}
	return err.Error()
}

type diagnosticResponse struct {
	http.ResponseWriter
	status   int
	bytes    int64
	writeErr error
}

func (w *diagnosticResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *diagnosticResponse) WriteHeader(status int) {
	if w.status == 0 && status >= 200 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *diagnosticResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += int64(n)
	if err != nil {
		w.writeErr = err
	}
	return n, err
}
func (w *diagnosticResponse) Flush() {
	_ = w.FlushError()
}
func (w *diagnosticResponse) FlushError() error {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil {
		w.writeErr = err
		return err
	}
	return nil
}

func logProxyRequest(w http.ResponseWriter, r *http.Request, client string) (http.ResponseWriter, *http.Request, func()) {
	id, started := rand.Text(), time.Now()
	r = r.WithContext(context.WithValue(r.Context(), requestLogKey{}, id))
	tracked := &diagnosticResponse{ResponseWriter: w}
	log.Printf("request=%s client=%s begin method=%s path=%q", id, client, r.Method, r.URL.EscapedPath())
	return tracked, r, func() {
		status := tracked.status
		if status == 0 {
			status = http.StatusOK
		}
		log.Printf("request=%s client=%s end status=%d bytes=%d duration_ms=%d context_error=%q write_error=%q",
			id, client, status, tracked.bytes, time.Since(started).Milliseconds(), diagnosticError(r.Context().Err()), diagnosticError(tracked.writeErr))
	}
}

type diagnosticBody struct {
	io.ReadCloser
	ctx                 context.Context
	request, attempt    string
	started             time.Time
	mu                  sync.Mutex
	bytes               int64
	firstRead, lastRead time.Time
	readErr             error
	eof                 bool
	once                sync.Once
}

func (b *diagnosticBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	now := time.Now()
	b.mu.Lock()
	b.bytes += int64(n)
	if n > 0 {
		if b.firstRead.IsZero() {
			b.firstRead = now
		}
		b.lastRead = now
	}
	if err == io.EOF {
		b.eof = true
	} else if err != nil {
		b.readErr = err
	}
	b.mu.Unlock()
	return n, err
}

func (b *diagnosticBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		firstByte, idle := int64(-1), int64(-1)
		if !b.firstRead.IsZero() {
			firstByte = b.firstRead.Sub(b.started).Milliseconds()
			idle = time.Since(b.lastRead).Milliseconds()
		}
		log.Printf("request=%s upstream=%s body_end bytes=%d duration_ms=%d first_byte_ms=%d idle_ms=%d eof=%t read_error=%q close_error=%q context_error=%q",
			b.request, b.attempt, b.bytes, time.Since(b.started).Milliseconds(), firstByte, idle, b.eof,
			diagnosticError(b.readErr), diagnosticError(err), diagnosticError(b.ctx.Err()))
	})
	return err
}

func (a *App) loggedRequest(request *http.Request) (*http.Response, error) {
	id, attempt, started := requestLogID(request.Context()), rand.Text(), time.Now()
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		log.Printf("request=%s upstream=%s connection remote=%q reused=%t idle=%t idle_ms=%d",
			id, attempt, info.Conn.RemoteAddr().String(), info.Reused, info.WasIdle, info.IdleTime.Milliseconds())
	}}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	log.Printf("request=%s upstream=%s begin method=%s host=%q path=%q",
		id, attempt, request.Method, request.URL.Hostname(), request.URL.EscapedPath())
	response, err := a.client.Do(request)
	if err != nil {
		log.Printf("request=%s upstream=%s transport_error=%q duration_ms=%d context_error=%q",
			id, attempt, diagnosticError(err), time.Since(started).Milliseconds(), diagnosticError(request.Context().Err()))
		return response, err
	}
	log.Printf("request=%s upstream=%s headers status=%d protocol=%q duration_ms=%d request_id=%q github_request_id=%q retry_after=%q",
		id, attempt, response.StatusCode, response.Proto, time.Since(started).Milliseconds(),
		response.Header.Get("X-Request-Id"), response.Header.Get("X-Github-Request-Id"), response.Header.Get("Retry-After"))
	response.Body = &diagnosticBody{ReadCloser: response.Body, ctx: request.Context(), request: id, attempt: attempt, started: started}
	return response, nil
}
