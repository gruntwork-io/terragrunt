package portal_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/portal"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catalogToken stands in for a credential an earlier login left on disk.
const catalogToken = "fake-access-token"

const twoRepositoriesBody = `{
	"repositories": [
		{"url": "https://github.com/acme/infrastructure-catalog"},
		{"url": "https://github.com/acme/service-catalog"}
	]
}`

const rateLimitBody = `{"error":"slow_down","error_description":"Too many requests."}`

// catalogAnswer is one answer a stub portal gives a catalog request.
type catalogAnswer struct {
	header http.Header
	body   string
	status int
}

// catalogStub answers requests in turn, repeating its last answer once the list
// runs out. It records the requests it answered and when each arrived, so a
// test can assert what was sent and how long the fetch waited between tries.
type catalogStub struct {
	answers  []catalogAnswer
	requests []*http.Request
	at       []time.Time
}

func (s *catalogStub) client() vhttp.Client {
	return vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		s.requests = append(s.requests, req)
		s.at = append(s.at, time.Now())

		answer := s.answers[min(len(s.at)-1, len(s.answers)-1)]

		header := http.Header{"Content-Type": {"application/json"}}
		maps.Copy(header, answer.header)

		return vhttp.Respond(answer.status, []byte(answer.body), header), nil
	})
}

// gaps is how long the fetch waited before each try, counting from since.
func (s *catalogStub) gaps(since time.Time) []time.Duration {
	gaps := make([]time.Duration, 0, len(s.at))

	for _, at := range s.at {
		gaps = append(gaps, at.Sub(since))
		since = at
	}

	return gaps
}

func servedWith(status int, body string) *catalogStub {
	return &catalogStub{answers: []catalogAnswer{{status: status, body: body}}}
}

func fetch(t *testing.T, c vhttp.Client) ([]portal.Repository, error) {
	t.Helper()

	return portal.FetchCatalog(t.Context(), logger.CreateLogger(), c, portalBaseURL, portal.Secret(catalogToken))
}

func TestFetchCatalogRequest(t *testing.T) {
	t.Parallel()

	stub := servedWith(http.StatusOK, twoRepositoriesBody)

	_, err := portal.FetchCatalog(
		t.Context(),
		logger.CreateLogger(),
		stub.client(),
		portalBaseURL+"/",
		portal.Secret(catalogToken),
	)
	require.NoError(t, err)
	require.Len(t, stub.requests, 1)

	got := stub.requests[0]

	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, portalBaseURL+"/api/v1/catalog/repositories", got.URL.String())
	assert.Equal(t, "Bearer fake-access-token", got.Header.Get("Authorization"))
	assert.Equal(t, "application/json", got.Header.Get("Accept"))
}

func TestFetchCatalogReturnsTheSelectedRepositories(t *testing.T) {
	t.Parallel()

	repositories, err := fetch(t, servedWith(http.StatusOK, twoRepositoriesBody).client())
	require.NoError(t, err)

	assert.Equal(t, []portal.Repository{
		{URL: "https://github.com/acme/infrastructure-catalog"},
		{URL: "https://github.com/acme/service-catalog"},
	}, repositories)
}

// TestFetchCatalogReadsAnUnfamiliarResponse pins that a portal adding fields to
// the catalog does not break a CLI that was released before them.
func TestFetchCatalogReadsAnUnfamiliarResponse(t *testing.T) {
	t.Parallel()

	body := `{
		"repositories": [
			{"url": "https://github.com/acme/infrastructure-catalog", "name": "Infrastructure", "tags": ["aws"]}
		],
		"total": 1,
		"next_page": null
	}`

	repositories, err := fetch(t, servedWith(http.StatusOK, body).client())
	require.NoError(t, err)

	assert.Equal(t, []portal.Repository{{URL: "https://github.com/acme/infrastructure-catalog"}}, repositories)
}

// TestFetchCatalogReturnsAnEmptyCatalog pins that an org which selected nothing
// is an answer rather than a failure.
func TestFetchCatalogReturnsAnEmptyCatalog(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name string
		body string
	}{
		{name: "an empty list", body: `{"repositories": []}`},
		{name: "no list at all", body: `{}`},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repositories, err := fetch(t, servedWith(http.StatusOK, tt.body).client())
			require.NoError(t, err)
			assert.Empty(t, repositories)
		})
	}
}

// TestFetchCatalogReportsARejectedCredential pins the error a caller matches to
// carry on as a user who has not logged in.
func TestFetchCatalogReportsARejectedCredential(t *testing.T) {
	t.Parallel()

	body := `{"error":"invalid_token","error_description":"The credential has expired."}`

	_, err := fetch(t, servedWith(http.StatusUnauthorized, body).client())
	require.ErrorIs(t, err, portal.ErrCredentialRejected)
	assert.NotErrorIs(t, err, portal.ErrNoHostedCatalog)
}

// TestFetchCatalogReportsAPortalWithNoCatalog pins the refusals a caller reads
// as a portal with nothing to serve: any 404 from the catalog path, including
// the HTML a proxy in front of the portal answers with, and the portal's own
// feature_not_enabled code.
func TestFetchCatalogReportsAPortalWithNoCatalog(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name   string
		body   string
		status int
	}{
		{
			name:   "the portal explains itself",
			status: http.StatusNotFound,
			body:   `{"error":"not_found","error_description":"No catalog."}`,
		},
		{
			name:   "something in front of the portal answers",
			status: http.StatusNotFound,
			body:   `<html>404 Not Found</html>`,
		},
		{
			name:   "the org does not have the catalog",
			status: http.StatusForbidden,
			body:   `{"error":"feature_not_enabled","error_description":"Not enabled."}`,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := fetch(t, servedWith(tt.status, tt.body).client())
			require.ErrorIs(t, err, portal.ErrNoHostedCatalog)
			assert.NotErrorIs(t, err, portal.ErrCredentialRejected)
		})
	}
}

// TestFetchCatalogWaitsOutARateLimit pins the wait between tries: the portal's
// own advice when it gave any, and the fetch's own backoff when it did not. An
// HTTP-date is advice the CLI does not read, so it falls back to the backoff
// rather than to an immediate retry.
func TestFetchCatalogWaitsOutARateLimit(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		header   http.Header
		wantGaps []time.Duration
	}{
		{
			name:     "no advice",
			wantGaps: []time.Duration{0, time.Second, 2 * time.Second},
		},
		{
			name:     "the portal asks for a wait",
			header:   http.Header{"Retry-After": {"4"}},
			wantGaps: []time.Duration{0, 4 * time.Second, 4 * time.Second},
		},
		{
			name:     "the portal asks for longer than the fetch will wait",
			header:   http.Header{"Retry-After": {"3600"}},
			wantGaps: []time.Duration{0, 10 * time.Second, 10 * time.Second},
		},
		{
			name:     "advice the CLI does not read",
			header:   http.Header{"Retry-After": {"Wed, 21 Oct 2026 07:28:00 GMT"}},
			wantGaps: []time.Duration{0, time.Second, 2 * time.Second},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				stub := &catalogStub{answers: []catalogAnswer{{
					status: http.StatusTooManyRequests,
					body:   rateLimitBody,
					header: tt.header,
				}}}

				start := time.Now()

				_, err := fetch(t, stub.client())
				require.Error(t, err)

				assert.Equal(t, tt.wantGaps, stub.gaps(start))
			})
		})
	}
}

func TestFetchCatalogReturnsTheCatalogAfterARateLimit(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &catalogStub{answers: []catalogAnswer{
			{status: http.StatusTooManyRequests, body: rateLimitBody, header: http.Header{"Retry-After": {"2"}}},
			{status: http.StatusOK, body: twoRepositoriesBody},
		}}

		start := time.Now()

		repositories, err := fetch(t, stub.client())
		require.NoError(t, err)

		assert.Len(t, repositories, 2)
		assert.Equal(t, []time.Duration{0, 2 * time.Second}, stub.gaps(start))
	})
}

// TestFetchCatalogReportsASustainedRateLimit pins that a limit which outlasts
// every try reaches the caller as its own class, and not as a portal serving no
// catalog.
func TestFetchCatalogReportsASustainedRateLimit(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &catalogStub{answers: []catalogAnswer{{
			status: http.StatusTooManyRequests,
			body:   rateLimitBody,
			header: http.Header{"Retry-After": {"4"}},
		}}}

		_, err := fetch(t, stub.client())

		var limited *portal.RateLimitedError
		require.ErrorAs(t, err, &limited)

		assert.Equal(t, 4*time.Second, limited.RetryAfter)
		require.NotErrorIs(t, err, portal.ErrNoHostedCatalog)
		require.NotErrorIs(t, err, portal.ErrCredentialRejected)
		assert.Len(t, stub.at, 3)
	})
}

func TestFetchCatalogStopsWaitingWhenTheContextIsCancelledWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		stub := servedWith(http.StatusTooManyRequests, rateLimitBody)

		go func() {
			synctest.Wait()
			cancel()
		}()

		_, err := portal.FetchCatalog(
			ctx,
			logger.CreateLogger(),
			stub.client(),
			portalBaseURL,
			portal.Secret(catalogToken),
		)
		require.ErrorIs(t, err, context.Canceled)
		assert.Len(t, stub.at, 1)
	})
}

// TestFetchCatalogReportsRefusal pins that a refusal the caller can do nothing
// about stays a refusal, including a 403 carrying no code of the portal's own.
func TestFetchCatalogReportsRefusal(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		body     string
		wantCode portal.ErrorCode
		status   int
	}{
		{
			name:   "something in front of the portal refuses",
			status: http.StatusForbidden,
			body:   `<html>403 Forbidden</html>`,
		},
		{
			name:     "the portal faulted",
			status:   http.StatusInternalServerError,
			body:     `{"error":"server_error","error_description":"Internal server error."}`,
			wantCode: portal.ErrorCodeServerError,
		},
		{
			name:   "something in front of the portal answered",
			status: http.StatusBadGateway,
			body:   `<html>502 Bad Gateway</html>`,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stub := servedWith(tt.status, tt.body)

			_, err := fetch(t, stub.client())

			var portalErr *portal.Error
			require.ErrorAs(t, err, &portalErr)

			assert.Equal(t, tt.wantCode, portalErr.Code)
			assert.Equal(t, tt.status, portalErr.StatusCode)
			assert.Len(t, stub.at, 1, "a refusal no retry can change is not sent again")
		})
	}
}

func TestFetchCatalogRejectsUnusableResponse(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name string
		body string
	}{
		{name: "not json", body: `<html>hello</html>`},
		{name: "truncated", body: `{"repositories": [{"url": "https://github.com/acme/`},
		{name: "wrong field type", body: `{"repositories": {"url": "https://github.com/acme/catalog"}}`},
		{name: "an entry that is not an object", body: `{"repositories": ["https://github.com/acme/catalog"]}`},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repositories, err := fetch(t, servedWith(http.StatusOK, tt.body).client())
			require.ErrorIs(t, err, portal.ErrMalformedResponse)
			assert.Nil(t, repositories)
		})
	}
}

// TestFetchCatalogRejectsAnEntryWithNoURL pins that a repository the portal
// named no URL for fails the fetch. Dropping it would hand the user a catalog
// silently one entry short.
func TestFetchCatalogRejectsAnEntryWithNoURL(t *testing.T) {
	t.Parallel()

	body := `{"repositories": [{"url": "https://github.com/acme/catalog"}, {"name": "Nowhere"}]}`

	_, err := fetch(t, servedWith(http.StatusOK, body).client())

	var missing *portal.MissingFieldError
	require.ErrorAs(t, err, &missing)

	assert.Equal(t, "repositories[].url", missing.Field)
	require.ErrorIs(t, err, portal.ErrMalformedResponse)
}

func TestFetchCatalogRejectsNonObjectResponse(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`null`, `[{"url":"https://github.com/acme/catalog"}]`, `"hello"`, `42`} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()

			_, err := fetch(t, servedWith(http.StatusOK, body).client())
			require.ErrorIs(t, err, portal.ErrResponseNotObject)
		})
	}
}

// TestFetchCatalogPropagatesTransportFailure pins that a portal the CLI got no
// answer out of reaches the caller as its own class. It is the offline user's
// case, and without a class of its own it would arrive as a refusal the caller
// stops at instead of one it falls back from.
func TestFetchCatalogPropagatesTransportFailure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("dial failed")

	c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return nil, sentinel
	})

	_, err := fetch(t, c)
	require.ErrorIs(t, err, sentinel)
	require.ErrorIs(t, err, portal.ErrPortalUnreachable)

	var portalErr *portal.Error
	assert.NotErrorAs(t, err, &portalErr)
	require.NotErrorIs(t, err, portal.ErrNoHostedCatalog)
	assert.NotErrorIs(t, err, portal.ErrCredentialRejected)
}

// TestFetchCatalogGivesUpOnAStalledPortal pins the bound on the whole fetch. The
// client the venv hands down carries no timeout, so a portal that accepts the
// connection and then says nothing would hold the command open until the user
// interrupted it.
func TestFetchCatalogGivesUpOnAStalledPortal(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		c := vhttp.NewMemClient(func(reqCtx context.Context, _ *http.Request) (*http.Response, error) {
			<-reqCtx.Done()

			return nil, reqCtx.Err()
		})

		start := time.Now()

		_, err := fetch(t, c)
		require.ErrorIs(t, err, portal.ErrPortalUnreachable)

		assert.Equal(t, 30*time.Second, time.Since(start))
	})
}

// TestFetchCatalogCarriesContext pins that the caller's context reaches the
// request, and that a caller who left is told so rather than told the portal
// could not be reached. Nothing about the portal ended this fetch.
func TestFetchCatalogCarriesContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	c := vhttp.NewMemClient(func(reqCtx context.Context, _ *http.Request) (*http.Response, error) {
		return nil, reqCtx.Err()
	})

	_, err := portal.FetchCatalog(ctx, logger.CreateLogger(), c, portalBaseURL, portal.Secret(catalogToken))
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, portal.ErrPortalUnreachable)
}

// TestFetchCatalogDoesNotFollowARedirect pins that the credential reaches the
// host the fetch addressed and no other. The stdlib copies an Authorization
// header onto a redirect naming the same hostname, so a portal answering with a
// downgrade to http would otherwise put the credential on the wire in the clear.
func TestFetchCatalogDoesNotFollowARedirect(t *testing.T) {
	t.Parallel()

	stub := &catalogStub{answers: []catalogAnswer{
		{
			status: http.StatusFound,
			header: http.Header{"Location": {"http://portal.example.com/api/v1/catalog/repositories"}},
		},
		{status: http.StatusOK, body: twoRepositoriesBody},
	}}

	_, err := fetch(t, stub.client())

	var portalErr *portal.Error
	require.ErrorAs(t, err, &portalErr)

	assert.Equal(t, http.StatusFound, portalErr.StatusCode)
	assert.Len(t, stub.requests, 1, "the credential is not sent on to the host a redirect names")
}

// TestFetchCatalogReportsTruncatedBodyAsRead pins that a connection dropping
// partway through the catalog is reported as the read failure it is. The
// decoder sees the same truncated document either way, so without the
// distinction a dropped connection reads as a portal serving malformed JSON.
func TestFetchCatalogReportsTruncatedBodyAsRead(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("connection reset by peer")

	c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(&truncatedReader{
				prefix: []byte(`{"repositories": [{"url": "https://github.com/acme/`),
				err:    sentinel,
			}),
		}, nil
	})

	_, err := fetch(t, c)
	require.ErrorIs(t, err, sentinel)
	require.NotErrorIs(t, err, portal.ErrMalformedResponse)
}

func TestFetchCatalogRejectsUnusableBaseURL(t *testing.T) {
	t.Parallel()

	stub := servedWith(http.StatusOK, twoRepositoriesBody)

	_, err := portal.FetchCatalog(
		t.Context(),
		logger.CreateLogger(),
		stub.client(),
		"://portal.example.com",
		portal.Secret(catalogToken),
	)
	require.Error(t, err)
	assert.Empty(t, stub.at, "an unusable base URL is caught before the request")
}

// TestFetchCatalogKeepsTheCredentialOutOfOutput pins that the credential leaves
// only through the Authorization header. Every refusal class is rendered here,
// through the logger as well as through fmt, because an error from a fetch is
// something a command goes on to print.
func TestFetchCatalogKeepsTheCredentialOutOfOutput(t *testing.T) {
	t.Parallel()

	statuses := []int{
		http.StatusUnauthorized,
		http.StatusNotFound,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
	}

	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				var buf bytes.Buffer

				l := logger.CreateLogger().WithOptions(log.WithOutput(&buf))

				stub := servedWith(status, `{"error":"server_error","error_description":"Internal server error."}`)

				repositories, err := portal.FetchCatalog(
					t.Context(),
					l,
					stub.client(),
					portalBaseURL,
					portal.Secret(catalogToken),
				)
				require.Error(t, err)
				assert.Nil(t, repositories)

				l.Errorf("fetching the catalog: %v", err)

				assert.NotContains(t, buf.String(), catalogToken)
				assert.NotContains(t, fmt.Sprintf("%v %+v %#v", err, err, err), catalogToken)

				require.NotEmpty(t, stub.requests)
				assert.Equal(t, "Bearer "+catalogToken, stub.requests[0].Header.Get("Authorization"))
			})
		})
	}
}
