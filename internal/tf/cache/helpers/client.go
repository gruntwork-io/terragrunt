package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	svchost "github.com/hashicorp/terraform-svchost"
	"golang.org/x/sync/singleflight"
)

const (
	// maxResponseBody bounds a registry response body length.
	maxResponseBody = 32 << 20

	// defaultRequestTimeout bounds a coalesced upstream metadata request when
	// the underlying http.Client leaves Timeout unset.
	defaultRequestTimeout = 60 * time.Second
)

// HTTPStatusError reports a transient HTTP failure from an upstream registry
// so callers can fail loudly rather than caching an empty response.
type HTTPStatusError struct {
	URL        string
	Status     string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("registry request %s returned HTTP %s", e.URL, e.Status)
}

// Client is the cache server's outbound HTTP client. It wraps a
// [vhttp.Client] with registry credential injection, in-flight request
// coalescing, and a per-URL response cache.
type Client struct {
	httpClient vhttp.Client

	credsSource *cliconfig.CredentialsSource
	cache       map[string][]byte
	group       singleflight.Group
	cacheMu     sync.Mutex
}

// NewClient returns a [Client] that dispatches requests through c.
// Pass [vhttp.NewOSClient] in production or a [vhttp.NewMemClient] in tests.
func NewClient(c vhttp.Client, credsSource *cliconfig.CredentialsSource) *Client {
	return &Client{
		httpClient:  c,
		credsSource: credsSource,
		cache:       make(map[string][]byte),
	}
}

// Do sends an HTTP request and decodes an HTTP response to the given `value`.
func (client *Client) Do(ctx context.Context, method, reqURL string, value any) error {
	if cached, ok := client.getCached(reqURL); ok {
		return unmarshalBody(cached, value)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	resCh := client.group.DoChan(method+" "+reqURL, func() (any, error) {
		if cached, ok := client.getCached(reqURL); ok {
			return cached, nil
		}

		reqCtx, cancel := client.requestContext(ctx)
		defer cancel()

		bodyBytes, err := client.fetch(reqCtx, method, reqURL)
		if err != nil {
			return nil, err
		}

		client.setCached(reqURL, bodyBytes)

		return bodyBytes, nil
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-resCh:
		if res.Err != nil {
			return res.Err
		}

		bodyBytes, _ := res.Val.([]byte)

		return unmarshalBody(bodyBytes, value)
	}
}

func (client *Client) getCached(reqURL string) ([]byte, bool) {
	client.cacheMu.Lock()
	defer client.cacheMu.Unlock()

	cached, ok := client.cache[reqURL]

	return cached, ok
}

func (client *Client) setCached(reqURL string, body []byte) {
	client.cacheMu.Lock()
	defer client.cacheMu.Unlock()

	client.cache[reqURL] = body
}

func (client *Client) requestContext(callerCtx context.Context) (context.Context, context.CancelFunc) {
	timeout := defaultRequestTimeout
	if client.httpClient != nil && client.httpClient.Timeout > 0 {
		timeout = client.httpClient.Timeout
	}

	return context.WithTimeout(context.WithoutCancel(callerCtx), timeout)
}

func (client *Client) fetch(ctx context.Context, method, reqURL string) (bodyBytes []byte, err error) {
	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, err
	}

	if client.credsSource != nil {
		hostname := svchost.Hostname(req.URL.Hostname())
		if creds := client.credsSource.ForHost(hostname); creds != nil {
			creds.PrepareRequest(req)
		}
	}

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	return decodeResponse(resp)
}

// ReadBody reads r into memory, refusing a body past limit with
// [ResponseTooLargeError] rather than decoding part of one.
func ReadBody(r io.Reader, hint, limit int64) ([]byte, error) {
	limited := io.LimitReader(r, limit+1)

	buf := &bytes.Buffer{}
	if hint > 0 && hint <= limit {
		buf.Grow(int(hint))
	}

	n, err := buf.ReadFrom(limited)
	if err != nil {
		return nil, err
	}

	if n > limit {
		return nil, ResponseTooLargeError{Limit: limit}
	}

	return buf.Bytes(), nil
}

func unmarshalBody(data []byte, value any) error {
	if data == nil {
		return nil
	}

	if err := json.Unmarshal(data, value); err != nil {
		return err
	}

	return nil
}

func decodeResponse(resp *http.Response) ([]byte, error) {
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		reqURL := ""
		if resp.Request != nil && resp.Request.URL != nil {
			reqURL = resp.Request.URL.String()
		}

		return nil, &HTTPStatusError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			URL:        reqURL,
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	reader, err := ResponseReader(resp)
	if err != nil {
		return nil, err
	}

	body, readErr := ReadBody(reader, resp.ContentLength, maxResponseBody)
	if err := errors.Join(readErr, reader.Close()); err != nil {
		return nil, err
	}

	return body, nil
}
