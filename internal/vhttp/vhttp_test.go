package vhttp_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestMemClient_HandlerReceivesRequest(t *testing.T) {
	t.Parallel()

	var got *http.Request

	c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		got = req
		return vhttp.Respond(http.StatusOK, []byte("ok"), nil), nil
	})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.test/foo", nil)
	require.NoError(t, err)

	resp, err := c.Do(req)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, resp.Body.Close())
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", string(body))
	assert.Equal(t, "https://example.test/foo", got.URL.String())
}

func TestMemClient_HandlerErrorPropagates(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("handler boom")

	c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return nil, sentinel
	})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.test", nil)
	require.NoError(t, err)

	resp, err := c.Do(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}

	require.ErrorIs(t, err, sentinel)
	assert.Nil(t, resp)
}

func TestMemClient_NilHandlerPanics(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		vhttp.NewMemClient(nil)
	})
}

func TestMemClient_HandlerReceivesContext(t *testing.T) {
	t.Parallel()

	type ctxKey string

	key := ctxKey("trace")

	c := vhttp.NewMemClient(func(ctx context.Context, _ *http.Request) (*http.Response, error) {
		assert.Equal(t, "abc", ctx.Value(key))
		return vhttp.Respond(http.StatusOK, nil, nil), nil
	})

	ctx := context.WithValue(t.Context(), key, "abc")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test", nil)
	require.NoError(t, err)

	resp, err := c.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestRespond(t *testing.T) {
	t.Parallel()

	headers := http.Header{"Content-Type": {"application/json"}}
	resp := vhttp.Respond(http.StatusCreated, []byte(`{"ok":true}`), headers)

	defer func() {
		require.NoError(t, resp.Body.Close())
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, http.StatusText(http.StatusCreated), resp.Status)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Equal(t, `{"ok":true}`, string(body))
	assert.Equal(t, int64(len(`{"ok":true}`)), resp.ContentLength)
}

func TestRespond_NilHeaders(t *testing.T) {
	t.Parallel()

	resp := vhttp.Respond(http.StatusNoContent, nil, nil)

	defer func() {
		require.NoError(t, resp.Body.Close())
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.NotNil(t, resp.Header)
	assert.Empty(t, body)
	assert.Equal(t, int64(0), resp.ContentLength)
}

func TestNewNoNetworkClient(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.test", nil)
	require.NoError(t, err)

	resp, err := vhttp.NewNoNetworkClient().Do(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}

	require.ErrorIs(t, err, vhttp.ErrNoNetwork)
	assert.Nil(t, resp)
}

func TestNewOSClient_SatisfiesClient(t *testing.T) {
	t.Parallel()

	c := vhttp.NewOSClient()
	require.NotNil(t, c)
}

func TestNewOSClientWithTimeout_AppliesTimeout(t *testing.T) {
	t.Parallel()

	c := vhttp.NewOSClientWithTimeout(7 * time.Second)
	assert.Equal(t, 7*time.Second, c.Timeout)
}

// TestMemClient_DoWithRacing exercises concurrent Do calls so CI runs it
// under -race. Naming suffix mandated by project convention.
func TestMemClient_DoWithRacing(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64

	c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		calls.Add(1)
		return vhttp.Respond(http.StatusOK, []byte("ok"), nil), nil
	})

	const workers = 16

	g, gctx := errgroup.WithContext(t.Context())

	for range workers {
		g.Go(func() error {
			req, err := http.NewRequestWithContext(gctx, http.MethodGet, "https://example.test", nil)
			if err != nil {
				return err
			}

			resp, err := c.Do(req)
			if err != nil {
				return err
			}

			return resp.Body.Close()
		})
	}

	require.NoError(t, g.Wait())
	assert.Equal(t, int64(workers), calls.Load())
}
