package bridge

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

type upstreamIdleError struct{}

func (upstreamIdleError) Error() string     { return "upstream response idle timeout" }
func (upstreamIdleError) Timeout() bool     { return true }
func (upstreamIdleError) Temporary() bool   { return true }
func (upstreamIdleError) Is(err error) bool { return err == context.DeadlineExceeded }

// Bound silence, not the total duration of an actively streaming response.
type idleTransport struct {
	base    *http.Transport
	timeout time.Duration
}

func (t *idleTransport) CloseIdleConnections() { t.base.CloseIdleConnections() }

func (t *idleTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancelCause(r.Context())
	response, err := t.base.RoundTrip(r.Clone(ctx))
	if err != nil {
		cancel(nil)
		return response, err
	}
	body := &idleBody{ReadCloser: response.Body, ctx: ctx, cancel: cancel, timeout: t.timeout, deadline: time.Now().Add(t.timeout)}
	body.mu.Lock()
	body.timer = time.AfterFunc(t.timeout, body.expire)
	body.mu.Unlock()
	response.Body = body
	return response, nil
}

type idleBody struct {
	io.ReadCloser
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelCauseFunc
	timer    *time.Timer
	timeout  time.Duration
	deadline time.Time
	err      error
	closed   bool
}

func (b *idleBody) expire() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.err != nil || b.ctx.Err() != nil {
		return
	}
	if remaining := time.Until(b.deadline); remaining > 0 {
		b.timer.Reset(remaining)
		return
	}
	b.err = upstreamIdleError{}
	b.cancel(b.err)
}

func (b *idleBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil && err != nil {
		err = b.err
	}
	if err != nil {
		b.timer.Stop()
	} else if n > 0 && !b.closed && b.err == nil {
		b.deadline = time.Now().Add(b.timeout)
		b.timer.Reset(b.timeout)
	}
	return n, err
}

func (b *idleBody) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.timer.Stop()
	b.cancel(nil)
	b.mu.Unlock()
	return b.ReadCloser.Close()
}
