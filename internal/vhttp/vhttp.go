// Package vhttp provides a virtual outbound-HTTP abstraction for testing
// and production use.
//
// [Client] is a type alias for *[net/http.Client]: the venv carries a
// single pointer (8 bytes) and consumers call the standard library's
// [net/http.Client.Do] directly. Test substitution happens at the
// transport boundary via [net/http.RoundTripper] — the established Go
// idiom — rather than through a parallel interface.
//
// Production code constructs the OS-backed [Client] via [NewOSClient]
// (or [NewOSClientWithTimeout] when a request timeout is required) and
// threads it down from [github.com/gruntwork-io/terragrunt/internal/venv.Venv].
// Tests construct an in-memory [Client] via [NewMemClient] with a
// [Handler] that synthesizes responses, eliminating any dependency on
// [net/http/httptest] servers or real network access.
package vhttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

// Client is the outbound-HTTP handle used throughout the codebase. It is
// a type alias for *[net/http.Client] so the venv carries a single pointer
// and consumers call [net/http.Client.Do] without an additional interface
// indirection.
type Client = *http.Client

// Handler responds to a request. It is invoked synchronously by the
// transport wired into an in-memory [Client] returned by [NewMemClient].
type Handler func(ctx context.Context, req *http.Request) (*http.Response, error)

// NewOSClient returns a [Client] backed by a fresh transport that
// disables keepalives and idle-connection reuse, matching what
// hashicorp/go-cleanhttp.DefaultClient produces.
func NewOSClient() Client {
	return &http.Client{Transport: newOSTransport()}
}

// NewOSClientWithTimeout returns a [Client] whose Timeout is set to d.
func NewOSClientWithTimeout(d time.Duration) Client {
	return &http.Client{Transport: newOSTransport(), Timeout: d}
}

// NewMemClient returns a [Client] whose every request is dispatched to h
// via a synthetic [net/http.RoundTripper] instead of the real network.
// h must not be nil.
func NewMemClient(h Handler) Client {
	if h == nil {
		panic("vhttp: NewMemClient requires a non-nil Handler")
	}

	return &http.Client{Transport: &memTransport{handler: h}}
}

// Respond builds a *[net/http.Response] suitable for returning from a
// [Handler]. The body is wrapped in an [io.NopCloser]; passing a nil body
// yields a zero-length body. Headers may be nil.
func Respond(status int, body []byte, headers http.Header) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}

	return &http.Response{
		StatusCode:    status,
		Status:        http.StatusText(status),
		Header:        headers,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

func newOSTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableKeepAlives = true
	t.MaxIdleConnsPerHost = -1

	return t
}

type memTransport struct {
	handler Handler
}

func (t *memTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.handler(req.Context(), req)
}
