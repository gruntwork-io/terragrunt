package helpers_test

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const versionsListing = `{"versions":[{"version":"5.0.0"},{"version":"5.1.0"}]}`

// testLimit stands in for the real body limit, which a test would otherwise
// have to allocate megabytes to reach.
const testLimit = 64

// TestReadBodyIgnoresTheHint pins that the body decides what comes back. A
// Content-Length is the hint's usual source and a registry picks that freely,
// so reading only as far as the hint would let one truncate a listing into a
// shorter listing that still parses.
func TestReadBodyIgnoresTheHint(t *testing.T) {
	t.Parallel()

	body := versionsListing

	for name, hint := range map[string]int64{
		"unknown length": -1,
		"declared empty": 0,
		"understated":    int64(len(body) / 2),
		"exact":          int64(len(body)),
		"overstated":     int64(len(body) * 2),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := helpers.ReadBody(strings.NewReader(body), hint, testLimit)
			require.NoError(t, err)
			assert.Equal(t, body, string(got))
		})
	}
}

// TestReadBodyRefusesABodyPastTheLimit pins that the limit bounds the read
// itself. A registry declaring nothing, or gzipping its response, arrives with
// no usable hint and would otherwise be read without a bound.
func TestReadBodyRefusesABodyPastTheLimit(t *testing.T) {
	t.Parallel()

	body := bytes.Repeat([]byte("x"), testLimit+1)

	_, err := helpers.ReadBody(bytes.NewReader(body), -1, testLimit)

	var tooLarge helpers.ResponseTooLargeError

	require.ErrorAs(t, err, &tooLarge)
	assert.Equal(t, int64(testLimit), tooLarge.Limit)
}

// TestReadBodyKeepsABodyOnTheLimit pins the boundary, so the limit is the
// largest body accepted rather than one byte below it.
func TestReadBodyKeepsABodyOnTheLimit(t *testing.T) {
	t.Parallel()

	body := bytes.Repeat([]byte("x"), testLimit)

	got, err := helpers.ReadBody(bytes.NewReader(body), testLimit, testLimit)
	require.NoError(t, err)
	assert.Len(t, got, testLimit)
}

// TestReadBodySurfacesATruncatedTransfer pins that a read ending early fails
// the call. The bytes that did arrive can parse as a shorter document, which
// would answer with versions the registry never published.
func TestReadBodySurfacesATruncatedTransfer(t *testing.T) {
	t.Parallel()

	cut := errors.New("connection reset")

	_, err := helpers.ReadBody(errAfter(versionsListing[:10], cut), -1, testLimit)
	require.ErrorIs(t, err, cut)
}

// TestClientDoDecodesAResponseItsContentLengthUnderstates pins the whole path
// for a header a registry can understate at will.
func TestClientDoDecodesAResponseItsContentLengthUnderstates(t *testing.T) {
	t.Parallel()

	body := []byte(versionsListing)

	c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		resp := vhttp.Respond(http.StatusOK, body, nil)
		resp.ContentLength = int64(len(body) / 2)

		return resp, nil
	})

	var value struct {
		Versions []struct {
			Version string `json:"version"`
		} `json:"versions"`
	}

	client := helpers.NewClient(c, nil)
	require.NoError(t, client.Do(
		t.Context(),
		http.MethodGet,
		"https://registry.test/v1/providers/hashicorp/aws/versions",
		&value,
	))

	assert.Len(t, value.Versions, 2)
}

// errAfter returns a reader that yields prefix and then fails, standing in for
// a transfer the transport reports as cut short.
func errAfter(prefix string, err error) *truncatedReader {
	return &truncatedReader{prefix: strings.NewReader(prefix), err: err}
}

type truncatedReader struct {
	prefix *strings.Reader
	err    error
}

func (r *truncatedReader) Read(p []byte) (int, error) {
	if r.prefix.Len() > 0 {
		return r.prefix.Read(p)
	}

	return 0, r.err
}

const versionsURL = "https://registry.test/v1/providers/hashicorp/aws/versions"

// versionsValue is what a caller decodes a versions listing into.
type versionsValue struct {
	Versions []struct {
		Version string `json:"version"`
	} `json:"versions"`
}

// TestClientDoSharesOneRequestAmongConcurrentCallersWithRacing pins that
// callers asking for the same URL at once cost the registry one request
// between them. Every unit of a `run --all` asks for the same provider
// listings, so without this the registry sees one request per unit and rate
// limits them. synctest parks every caller before the held request is
// released, so the sharing is observed rather than assumed.
func TestClientDoSharesOneRequestAmongConcurrentCallersWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const callers = 32

		var hits atomic.Int32

		release := make(chan struct{})

		client := helpers.NewClient(vhttp.NewMemClient(
			func(_ context.Context, _ *http.Request) (*http.Response, error) {
				hits.Add(1)
				<-release

				return vhttp.Respond(http.StatusOK, []byte(versionsListing), nil), nil
			},
		), nil)

		values := make([]versionsValue, callers)
		errs := make([]error, callers)

		var wg sync.WaitGroup

		for i := range callers {
			wg.Go(func() {
				errs[i] = client.Do(t.Context(), http.MethodGet, versionsURL, &values[i])
			})
		}

		synctest.Wait()
		close(release)
		wg.Wait()

		assert.Equal(t, int32(1), hits.Load(), "every caller must be waiting on one registry request")

		for i := range callers {
			require.NoError(t, errs[i], "caller %d", i)
			assert.Len(t, values[i].Versions, 2, "caller %d must get the shared answer", i)
		}
	})
}

// TestClientDoDoesNotRememberARefusal pins that an answer the registry gave
// while it was refusing requests is not kept for the URL. Remembering it made
// one rate-limited answer stand for the provider for the rest of the run, so
// every later unit was told the provider had no versions.
func TestClientDoDoesNotRememberARefusal(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// Every try the first call makes is refused, then the registry recovers.
		stub := &registryStub{answers: []registryAnswer{
			{status: http.StatusTooManyRequests},
			{status: http.StatusTooManyRequests},
			{status: http.StatusTooManyRequests},
			{status: http.StatusTooManyRequests},
			{status: http.StatusOK, body: versionsListing},
		}}

		client := helpers.NewClient(stub.client(), nil)

		var refused versionsValue

		err := client.Do(t.Context(), http.MethodGet, versionsURL, &refused)

		var statusErr *helpers.StatusError

		require.ErrorAs(t, err, &statusErr, "a refusal must reach the caller as an error, not as an empty answer")
		assert.Empty(t, refused.Versions)

		var value versionsValue

		require.NoError(t, client.Do(t.Context(), http.MethodGet, versionsURL, &value))
		assert.Len(t, value.Versions, 2, "a later call must ask the registry again")
		assert.Equal(t, 5, stub.requests())
	})
}

// TestClientDoRetriesARefusalForNow pins which answers are retried: those a
// registry, or a proxy or CDN in front of it, sends while it rate limits or
// fails for a moment. Every try is refused here, so the caller gets the last
// refusal once the tries run out. Any other status is the registry's final
// answer and is not retried. The error names the status, with its text when
// the status has a standard one.
func TestClientDoRetriesARefusalForNow(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name          string
		wantStatus    string
		status        int
		wantRequests  int
		wantTransient bool
	}{
		{
			name:          "rate limited",
			status:        http.StatusTooManyRequests,
			wantStatus:    "429 Too Many Requests",
			wantRequests:  4,
			wantTransient: true,
		},
		{
			name:          "internal server error",
			status:        http.StatusInternalServerError,
			wantStatus:    "500 Internal Server Error",
			wantRequests:  4,
			wantTransient: true,
		},
		{
			name:          "bad gateway",
			status:        http.StatusBadGateway,
			wantStatus:    "502 Bad Gateway",
			wantRequests:  4,
			wantTransient: true,
		},
		{
			name:          "unavailable",
			status:        http.StatusServiceUnavailable,
			wantStatus:    "503 Service Unavailable",
			wantRequests:  4,
			wantTransient: true,
		},
		{
			name:          "gateway timeout",
			status:        http.StatusGatewayTimeout,
			wantStatus:    "504 Gateway Timeout",
			wantRequests:  4,
			wantTransient: true,
		},
		{
			// A CDN status with no standard text, such as Cloudflare's
			// "connection timed out" to the origin.
			name:          "a CDN cannot reach the registry",
			status:        522,
			wantStatus:    "522",
			wantRequests:  4,
			wantTransient: true,
		},
		{
			name:         "not implemented",
			status:       http.StatusNotImplemented,
			wantStatus:   "501 Not Implemented",
			wantRequests: 1,
		},
		{
			name:         "unauthorized",
			status:       http.StatusUnauthorized,
			wantStatus:   "401 Unauthorized",
			wantRequests: 1,
		},
		{
			name:         "forbidden",
			status:       http.StatusForbidden,
			wantStatus:   "403 Forbidden",
			wantRequests: 1,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				stub := &registryStub{answers: []registryAnswer{{status: tt.status}}}
				client := helpers.NewClient(stub.client(), nil)

				var value versionsValue

				err := client.Do(t.Context(), http.MethodGet, versionsURL, &value)

				var statusErr *helpers.StatusError

				require.ErrorAs(t, err, &statusErr)
				assert.Equal(t, tt.status, statusErr.StatusCode)
				assert.Equal(t, versionsURL, statusErr.URL)
				assert.Equal(t, tt.wantTransient, statusErr.Transient())
				assert.Equal(t, tt.wantRequests, stub.requests())
				assert.Equal(t, versionsURL+" returned "+tt.wantStatus, err.Error())

				// Not remembered either way: the next call asks again.
				require.Error(t, client.Do(t.Context(), http.MethodGet, versionsURL, &value))
				assert.Equal(t, 2*tt.wantRequests, stub.requests())
			})
		})
	}
}

// TestClientDoWaitsOutARefusal pins the wait between tries: the registry's
// own Retry-After when it gave one, capped so one answer cannot stall a run,
// and a doubling backoff when it did not. Retry-After may be a number of
// seconds or an HTTP-date (RFC 9110, section 10.2.3).
func TestClientDoWaitsOutARefusal(t *testing.T) {
	t.Parallel()

	// synctest's clock starts at midnight UTC on 2000-01-01, so a date three
	// seconds after it is the first try plus three seconds.
	threeSecondsIn := time.Date(2000, time.January, 1, 0, 0, 3, 0, time.UTC).Format(http.TimeFormat)

	backoff := []time.Duration{0, 500 * time.Millisecond, time.Second, 2 * time.Second}

	tc := []struct {
		name           string
		retryAfter     string
		wantGaps       []time.Duration
		wantRetryAfter time.Duration
	}{
		{
			name:     "no advice",
			wantGaps: backoff,
		},
		{
			name:           "the registry asks for a wait in seconds",
			retryAfter:     "2",
			wantGaps:       []time.Duration{0, 2 * time.Second, 2 * time.Second, 2 * time.Second},
			wantRetryAfter: 2 * time.Second,
		},
		{
			name:           "the registry asks for longer than the client waits",
			retryAfter:     "3600",
			wantGaps:       []time.Duration{0, 10 * time.Second, 10 * time.Second, 10 * time.Second},
			wantRetryAfter: 10 * time.Second,
		},
		{
			name:           "a count too large for a duration",
			retryAfter:     "99999999999999999999",
			wantGaps:       []time.Duration{0, 10 * time.Second, 10 * time.Second, 10 * time.Second},
			wantRetryAfter: 10 * time.Second,
		},
		{
			// The date is three seconds after the first try. By the second
			// try it has passed, so the backoff takes over from there.
			name:       "the registry asks for a wait until a date",
			retryAfter: threeSecondsIn,
			wantGaps:   []time.Duration{0, 3 * time.Second, time.Second, 2 * time.Second},
		},
		{
			name:       "a negative wait",
			retryAfter: "-5",
			wantGaps:   backoff,
		},
		{
			name:       "advice that is neither a count nor a date",
			retryAfter: "soon",
			wantGaps:   backoff,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				answer := registryAnswer{status: http.StatusTooManyRequests}
				if tt.retryAfter != "" {
					answer.header = http.Header{"Retry-After": {tt.retryAfter}}
				}

				stub := &registryStub{answers: []registryAnswer{answer}}
				client := helpers.NewClient(stub.client(), nil)

				start := time.Now()

				var value versionsValue

				err := client.Do(t.Context(), http.MethodGet, versionsURL, &value)

				var statusErr *helpers.StatusError

				require.ErrorAs(t, err, &statusErr)
				assert.Equal(t, tt.wantGaps, stub.gaps(start))

				// The last try was made at least a second after the date, so
				// only a count is still advice by then.
				assert.Equal(t, tt.wantRetryAfter, statusErr.RetryAfter)
			})
		})
	}
}

// TestClientDoReturnsTheAnswerOnceTheRegistryRecovers pins the case retrying
// exists for: a registry that refuses a request for a moment and then answers
// it. The caller gets the answer, and only the registry's wait was spent.
func TestClientDoReturnsTheAnswerOnceTheRegistryRecovers(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &registryStub{answers: []registryAnswer{
			{status: http.StatusTooManyRequests, header: http.Header{"Retry-After": {"2"}}},
			{status: http.StatusOK, body: versionsListing},
		}}

		client := helpers.NewClient(stub.client(), nil)

		start := time.Now()

		var value versionsValue

		require.NoError(t, client.Do(t.Context(), http.MethodGet, versionsURL, &value))
		assert.Len(t, value.Versions, 2)
		assert.Equal(t, []time.Duration{0, 2 * time.Second}, stub.gaps(start))
	})
}

// TestClientDoGivesUpOnARegistryThatNeverAnswers pins that a shared request
// ends on its own. It runs apart from the callers waiting on it and the
// client underneath has no timeout, so a registry that accepts the request
// and never answers would otherwise hold it, and a slot, for good. Nothing is
// remembered, so a later call asks again.
func TestClientDoGivesUpOnARegistryThatNeverAnswers(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var hits atomic.Int32

		client := helpers.NewClient(vhttp.NewMemClient(
			func(ctx context.Context, _ *http.Request) (*http.Response, error) {
				hits.Add(1)
				<-ctx.Done()

				return nil, ctx.Err()
			},
		), nil)

		start := time.Now()

		var value versionsValue

		err := client.Do(t.Context(), http.MethodGet, versionsURL, &value)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, time.Minute, time.Since(start))

		require.ErrorIs(t, client.Do(t.Context(), http.MethodGet, versionsURL, &value), context.DeadlineExceeded)
		assert.Equal(t, int32(2), hits.Load(), "a request that timed out must not be remembered")
	})
}

// TestClientDoRemembersANotFound pins that a 404 stays a quiet answer: no
// error, the value left as it was, and no second request for the URL. A
// network mirror that lacks a provider answers this way, and the server then
// goes on to the next installation method.
func TestClientDoRemembersANotFound(t *testing.T) {
	t.Parallel()

	stub := &registryStub{answers: []registryAnswer{{status: http.StatusNotFound}}}
	client := helpers.NewClient(stub.client(), nil)

	for range 2 {
		var value versionsValue

		require.NoError(t, client.Do(t.Context(), http.MethodGet, versionsURL, &value))
		assert.Empty(t, value.Versions)
	}

	assert.Equal(t, 1, stub.requests())
}

// TestClientDoCapsRequestsInFlightWithRacing pins that one client never has
// more than eight requests in flight. Sharing a request only helps callers
// who ask for the same URL, so a `run --all` whose units need many different
// providers would otherwise still send the registry one burst per provider.
func TestClientDoCapsRequestsInFlightWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const (
			urls     = 40
			wantPeak = 8
		)

		var (
			inFlight atomic.Int32
			peak     atomic.Int32
		)

		release := make(chan struct{})

		client := helpers.NewClient(vhttp.NewMemClient(
			func(_ context.Context, _ *http.Request) (*http.Response, error) {
				n := inFlight.Add(1)
				defer inFlight.Add(-1)

				for {
					seen := peak.Load()
					if n <= seen || peak.CompareAndSwap(seen, n) {
						break
					}
				}

				<-release

				return vhttp.Respond(http.StatusOK, []byte(versionsListing), nil), nil
			},
		), nil)

		errs := make([]error, urls)

		var wg sync.WaitGroup

		for i := range urls {
			wg.Go(func() {
				var value versionsValue

				reqURL := "https://registry.test/v1/providers/example/p" + strconv.Itoa(i) + "/versions"
				errs[i] = client.Do(t.Context(), http.MethodGet, reqURL, &value)
			})
		}

		synctest.Wait()
		assert.Equal(t, int32(wantPeak), inFlight.Load(), "the cap must hold back the rest while it is reached")

		close(release)
		wg.Wait()

		assert.Equal(t, int32(wantPeak), peak.Load())

		for i := range urls {
			require.NoError(t, errs[i], "request %d must still be served once a slot frees", i)
		}
	})
}

// TestClientDoLetsACallerLeaveWithoutFailingTheOthersWithRacing pins that the
// shared request does not belong to the caller who started it. A caller whose
// context ends returns at once, while the request goes on for every other
// caller still waiting on it, and its answer is remembered for later ones.
func TestClientDoLetsACallerLeaveWithoutFailingTheOthersWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var hits atomic.Int32

		release := make(chan struct{})

		client := helpers.NewClient(vhttp.NewMemClient(
			func(ctx context.Context, _ *http.Request) (*http.Response, error) {
				hits.Add(1)

				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}

				return vhttp.Respond(http.StatusOK, []byte(versionsListing), nil), nil
			},
		), nil)

		firstCtx, cancelFirst := context.WithCancel(t.Context())
		defer cancelFirst()

		var (
			secondErr error
			second    versionsValue
			wg        sync.WaitGroup
		)

		firstErr := make(chan error, 1)

		wg.Go(func() {
			var value versionsValue

			firstErr <- client.Do(firstCtx, http.MethodGet, versionsURL, &value)
		})

		// The first caller's request is under way before the second asks.
		synctest.Wait()

		wg.Go(func() {
			secondErr = client.Do(t.Context(), http.MethodGet, versionsURL, &second)
		})

		synctest.Wait()
		cancelFirst()
		synctest.Wait()

		select {
		case err := <-firstErr:
			require.ErrorIs(t, err, context.Canceled, "the caller that left must get its own context's error")
		default:
			require.FailNow(t, "the caller that left must return while the request is still held")
		}

		close(release)
		wg.Wait()

		require.NoError(t, secondErr, "a caller still waiting must not inherit the other caller's cancellation")
		assert.Len(t, second.Versions, 2)

		var later versionsValue

		require.NoError(t, client.Do(t.Context(), http.MethodGet, versionsURL, &later))
		assert.Len(t, later.Versions, 2)
		assert.Equal(t, int32(1), hits.Load(), "both callers and the later one must share one registry request")
	})
}

// registryAnswer is one response a [registryStub] gives.
type registryAnswer struct {
	header http.Header
	body   string
	status int
}

// registryStub answers requests with answers in order, repeating the last
// one, and records when each request arrived.
type registryStub struct {
	answers []registryAnswer
	at      []time.Time
	mu      sync.Mutex
}

func (s *registryStub) client() vhttp.Client {
	return vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.at = append(s.at, time.Now())
		answer := s.answers[min(len(s.at)-1, len(s.answers)-1)]

		header := http.Header{"Content-Type": {"application/json"}}
		maps.Copy(header, answer.header)

		return vhttp.Respond(answer.status, []byte(answer.body), header), nil
	})
}

// requests is how many requests reached the registry.
func (s *registryStub) requests() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.at)
}

// gaps is how long the client waited before each request, counting from since.
func (s *registryStub) gaps(since time.Time) []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	gaps := make([]time.Duration, 0, len(s.at))

	for _, at := range s.at {
		gaps = append(gaps, at.Sub(since))
		since = at
	}

	return gaps
}
