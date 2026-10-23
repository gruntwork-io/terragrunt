package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/retry"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	svchost "github.com/hashicorp/terraform-svchost"
	"golang.org/x/sync/singleflight"
)

const (
	// maxResponseBody bounds a registry response body length.
	maxResponseBody = 32 << 20

	// maxRequestsInFlight bounds the requests one [Client] has in flight at
	// once. Callers asking for the same URL already share a request, so this
	// bounds the burst a `run --all` sends when its units need many different
	// providers.
	maxRequestsInFlight = 8

	// maxAttempts bounds the tries a request the registry refuses for now
	// makes before the refusal is returned.
	maxAttempts = 4

	// retryBackoff is the wait before a second try when the registry named no
	// wait of its own, doubling on each further try.
	retryBackoff = 500 * time.Millisecond

	// maxRetryAfter caps the wait a registry's Retry-After can ask for, so one
	// answer cannot stall every unit waiting on the request.
	maxRetryAfter = 10 * time.Second

	// fetchTimeout bounds one shared request, tries and waits included. It
	// runs apart from the callers waiting on it, and the client underneath
	// carries no timeout of its own.
	fetchTimeout = time.Minute
)

// StatusError reports a registry answer a request cannot use. [Client.Do]
// returns it for any status other than 200 OK or 404 Not Found.
type StatusError struct {
	// URL is the request the registry answered.
	URL string

	// RetryAfter is how long the registry asked callers to wait before trying
	// again, from its Retry-After header, capped at 10 seconds. It is zero
	// when the registry named no wait.
	RetryAfter time.Duration

	// StatusCode is the HTTP status the registry answered with.
	StatusCode int
}

// NewStatusError describes resp, the registry's answer to reqURL, as a
// [*StatusError], reading the wait it asked for from its Retry-After header.
func NewStatusError(reqURL string, resp *http.Response) *StatusError {
	return &StatusError{
		URL:        reqURL,
		StatusCode: resp.StatusCode,
		RetryAfter: retryAfter(resp.Header, time.Now()),
	}
}

func (err *StatusError) Error() string {
	if text := http.StatusText(err.StatusCode); text != "" {
		return fmt.Sprintf("%s returned %d %s", err.URL, err.StatusCode, text)
	}

	return fmt.Sprintf("%s returned %d", err.URL, err.StatusCode)
}

// Transient reports whether the registry refused the request for now: it is
// rate limiting (429), or it, or a proxy or CDN in front of it, failed with a
// server error other than 501 Not Implemented. The same request can succeed
// later. go-retryablehttp's default retry policy retries these as well.
func (err *StatusError) Transient() bool {
	if err.StatusCode == http.StatusTooManyRequests {
		return true
	}

	return err.StatusCode >= http.StatusInternalServerError &&
		err.StatusCode < 600 &&
		err.StatusCode != http.StatusNotImplemented
}

// Client is the cache server's outbound HTTP client. It wraps a
// [vhttp.Client] with registry credential injection, a per-URL response
// cache, request sharing between concurrent callers, retries of requests a
// registry refuses for now, and a cap on the requests in flight.
type Client struct {
	httpClient vhttp.Client

	credsSource *cliconfig.CredentialsSource
	cache       map[string][]byte

	// slots holds a token for each request in flight.
	slots chan struct{}

	flights singleflight.Group
	cacheMu sync.Mutex
}

// NewClient returns a [Client] that dispatches requests through c.
// Pass [vhttp.NewOSClient] in production or a [vhttp.NewMemClient] in tests.
func NewClient(c vhttp.Client, credsSource *cliconfig.CredentialsSource) *Client {
	return &Client{
		httpClient:  c,
		credsSource: credsSource,
		cache:       make(map[string][]byte),
		slots:       make(chan struct{}, maxRequestsInFlight),
	}
}

// Do sends an HTTP request and decodes an HTTP response to the given `value`.
//
// Concurrent calls for the same URL share one request. Its answer is
// remembered for later calls when it is final: the body of a 200 OK, or a 404
// Not Found, which leaves value as it is and returns no error. Any other
// status returns a [*StatusError] and is not remembered, so a later call asks
// the registry again. A refusal for now (see [StatusError.Transient]) is
// retried a few times first, waiting as long as the registry asks.
//
// A caller whose ctx ends gets ctx.Err() at once. The shared request goes on
// for the other callers, and its answer is still remembered.
func (client *Client) Do(ctx context.Context, method, reqURL string, value any) error {
	if body, ok := client.load(reqURL); ok {
		return unmarshalBody(body, value)
	}

	// The shared request must not end with the caller that happened to start
	// it, so it runs under a context that keeps ctx's values but not its
	// cancellation.
	sharedCtx := context.WithoutCancel(ctx)

	result := client.flights.DoChan(reqURL, func() (any, error) {
		return client.fetch(sharedCtx, method, reqURL)
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-result:
		if res.Err != nil {
			return res.Err
		}

		body, _ := res.Val.([]byte)

		return unmarshalBody(body, value)
	}
}

// fetch requests reqURL, trying again while the registry refuses for now, and
// remembers a final answer.
func (client *Client) fetch(ctx context.Context, method, reqURL string) ([]byte, error) {
	// A caller that missed the cache just before the previous request for
	// this URL stored its answer starts a request of its own.
	if body, ok := client.load(reqURL); ok {
		return body, nil
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	var body []byte

	err := RetryRefusals(ctx, func(ctx context.Context) error {
		var err error

		body, err = client.fetchOnce(ctx, method, reqURL)

		return err
	})
	if err != nil {
		return nil, err
	}

	client.cacheMu.Lock()
	client.cache[reqURL] = body
	client.cacheMu.Unlock()

	return body, nil
}

// fetchOnce makes one request once a slot is free. It returns the body of a
// 200 OK, nil for a 404 Not Found, and a [*StatusError] for anything else.
func (client *Client) fetchOnce(ctx context.Context, method, reqURL string) (body []byte, err error) {
	select {
	case client.slots <- struct{}{}:
		defer func() { <-client.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

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

	switch resp.StatusCode {
	case http.StatusOK:
		return decodeResponse(resp)
	case http.StatusNotFound:
		return nil, nil
	}

	return nil, NewStatusError(reqURL, resp)
}

func (client *Client) load(reqURL string) ([]byte, bool) {
	client.cacheMu.Lock()
	defer client.cacheMu.Unlock()

	body, ok := client.cache[reqURL]

	return body, ok
}

// RetryRefusals calls fn until it succeeds, until it fails with anything other
// than a [*StatusError] sent while the registry refuses requests for now (see
// [StatusError.Transient]), or until it has been tried four times. Between
// tries it waits as long as the registry asked, up to 10 seconds, or backs off
// from half a second when the registry named no wait. It returns fn's last
// error, or ctx's error when ctx ends during a wait.
func RetryRefusals(ctx context.Context, fn func(ctx context.Context) error) error {
	return retry.Do(ctx, maxAttempts, fn, func(attempt int, err error) (time.Duration, bool) {
		var statusErr *StatusError
		if !errors.As(err, &statusErr) || !statusErr.Transient() {
			return 0, false
		}

		return retryDelay(statusErr, attempt), true
	})
}

// retryDelay is how long to wait before trying a refused request again: the
// registry's own advice when it gave any, and otherwise a wait that doubles
// with each attempt.
func retryDelay(refusal *StatusError, attempt int) time.Duration {
	if refusal.RetryAfter > 0 {
		return refusal.RetryAfter
	}

	return retryBackoff * time.Duration(1<<(attempt-1))
}

// retryAfter reads a Retry-After header in either form RFC 9110 (section
// 10.2.3) allows, a number of seconds or an HTTP-date, which is measured from
// now. The wait is capped at [maxRetryAfter]. A missing or unreadable header,
// or one that asks for no wait, reads as zero.
func retryAfter(header http.Header, now time.Time) time.Duration {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if value == "" {
		return 0
	}

	// ParseUint refuses a sign, and reports a count too large for its type as
	// a range error alongside the largest value, which the cap then applies to.
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil || errors.Is(err, strconv.ErrRange) {
		if seconds >= uint64(maxRetryAfter/time.Second) {
			return maxRetryAfter
		}

		return time.Duration(seconds) * time.Second
	}

	date, err := http.ParseTime(value)
	if err != nil {
		return 0
	}

	return min(max(date.Sub(now), 0), maxRetryAfter)
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
