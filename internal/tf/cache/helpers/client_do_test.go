package helpers_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientDoCachesResponsesPerURL(t *testing.T) {
	t.Parallel()

	listings := []struct {
		calls   *atomic.Int32
		url     string
		listing string
		want    []string
	}{
		{
			calls:   new(atomic.Int32),
			url:     "https://registry.test/aws/versions",
			listing: versionsListing,
			want:    []string{"5.0.0", "5.1.0"},
		},
		{
			calls:   new(atomic.Int32),
			url:     "https://registry.test/google/versions",
			listing: `{"versions":[{"version":"6.0.0"}]}`,
			want:    []string{"6.0.0"},
		},
		{
			calls:   new(atomic.Int32),
			url:     "https://registry2.test/aws/versions",
			listing: `{"versions":[{"version":"7.0.0"}]}`,
			want:    []string{"7.0.0"},
		},
	}

	c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		for _, l := range listings {
			if l.url == req.URL.String() {
				l.calls.Add(1)

				return vhttp.Respond(http.StatusOK, []byte(l.listing), nil), nil
			}
		}

		return vhttp.Respond(http.StatusNotFound, nil, nil), nil
	})

	client := helpers.NewClient(c, nil)

	for range 2 {
		for _, l := range listings {
			var value versionsValue

			require.NoError(t, client.Do(t.Context(), http.MethodGet, l.url, &value))

			got := make([]string, 0, len(value.Versions))
			for _, v := range value.Versions {
				got = append(got, v.Version)
			}

			assert.Equal(t, l.want, got, "each url should decode its own listing")
		}
	}

	for _, l := range listings {
		assert.Equal(t, int32(1), l.calls.Load(), "%s should be fetched once and then served from the cache", l.url)
	}
}

func TestClientDoSendsHostCredentials(t *testing.T) {
	t.Parallel()

	var authorization atomic.Value

	c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		authorization.Store(req.Header.Get("Authorization"))

		return vhttp.Respond(http.StatusOK, []byte(versionsListing), nil), nil
	})

	creds := (&cliconfig.Config{}).CredentialsSource(map[string]string{"TF_TOKEN_registry_test": "registry-token"})

	var value versionsValue

	require.NoError(
		t,
		helpers.NewClient(c, creds).Do(t.Context(), http.MethodGet, "https://registry.test/versions", &value),
	)
	assert.Equal(t, "Bearer registry-token", authorization.Load(), "the request should carry the token for its host")
}

func TestClientDoLeavesValueEmptyForNonOKResponse(t *testing.T) {
	t.Parallel()

	c := vhttp.NewMemClient(func(context.Context, *http.Request) (*http.Response, error) {
		return vhttp.Respond(http.StatusNotFound, []byte(versionsListing), nil), nil
	})

	var value versionsValue

	require.NoError(t, helpers.NewClient(c, nil).Do(t.Context(), http.MethodGet, "https://registry.test/versions", &value))
	assert.Empty(t, value.Versions, "a failed response should not be decoded")
}

func TestClientDoFailures(t *testing.T) {
	t.Parallel()

	cut := errors.New("connection reset")
	errClose := errors.New("close failed")

	testCases := []struct {
		respond   vhttp.Handler
		assertErr func(t *testing.T, err error)
		name      string
		reqURL    string
	}{
		{
			name:   "url that does not parse",
			reqURL: "https://registry.test/%zz",
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var urlErr *url.Error

				require.ErrorAs(t, err, &urlErr)
			},
		},
		{
			name: "transport failure",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				return nil, cut
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, cut)
			},
		},
		{
			name: "body that is not gzip despite its encoding",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				return vhttp.Respond(http.StatusOK, []byte(notGzip), http.Header{"Content-Encoding": []string{"gzip"}}), nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, gzip.ErrHeader)
			},
		},
		{
			name: "transfer cut short",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				resp := vhttp.Respond(http.StatusOK, nil, nil)
				resp.Body = io.NopCloser(errAfter(versionsListing[:10], cut))

				return resp, nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, cut)
			},
		},
		{
			name: "body that is not json",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				return vhttp.Respond(http.StatusOK, []byte("not json"), nil), nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var syntaxErr *json.SyntaxError

				require.ErrorAs(t, err, &syntaxErr)
			},
		},
		{
			name: "body that fails to close",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				resp := vhttp.Respond(http.StatusOK, nil, nil)
				resp.Body = failingCloseBody{Reader: strings.NewReader(versionsListing), err: errClose}

				return resp, nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errClose)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			respond := tc.respond
			if respond == nil {
				respond = func(context.Context, *http.Request) (*http.Response, error) {
					return vhttp.Respond(http.StatusOK, []byte(versionsListing), nil), nil
				}
			}

			reqURL := tc.reqURL
			if reqURL == "" {
				reqURL = "https://registry.test/versions"
			}

			var value versionsValue

			tc.assertErr(t, helpers.NewClient(vhttp.NewMemClient(respond), nil).Do(t.Context(), http.MethodGet, reqURL, &value))
		})
	}
}

func TestClientDoCoalescesConcurrentRequestsAndDoesNotCacheTransientErrors(t *testing.T) {
	t.Parallel()

	t.Run("coalesces concurrent misses and shields peers from one canceled caller", func(t *testing.T) {
		t.Parallel()

		var hits atomic.Int32

		started := make(chan struct{})
		release := make(chan struct{})

		var once, releaseOnce sync.Once

		t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

		c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
			hits.Add(1)
			once.Do(func() { close(started) })
			<-release

			return vhttp.Respond(http.StatusOK, []byte(versionsListing), nil), nil
		})

		client := helpers.NewClient(c, nil)

		const callers = 16

		leaderCtx, cancelLeader := context.WithCancel(t.Context())
		defer cancelLeader()

		leaderErr := make(chan error, 1)

		go func() {
			var v versionsValue

			leaderErr <- client.Do(leaderCtx, http.MethodGet, "https://registry.test/v1/providers/hashicorp/aws/versions", &v)
		}()

		<-started

		followerErrs := make(chan error, callers-1)

		var wg sync.WaitGroup
		for range callers - 1 {
			wg.Go(func() {
				var v versionsValue

				err := client.Do(
					t.Context(),
					http.MethodGet,
					"https://registry.test/v1/providers/hashicorp/aws/versions",
					&v,
				)
				if err == nil && len(v.Versions) != 2 {
					err = errors.New("expected 2 decoded versions")
				}

				followerErrs <- err
			})
		}

		// Cancel the leader while followers are queued; followers must still complete from the shared flight.
		cancelLeader()
		require.ErrorIs(t, <-leaderErr, context.Canceled)

		releaseOnce.Do(func() { close(release) })
		wg.Wait()
		close(followerErrs)

		for err := range followerErrs {
			require.NoError(t, err)
		}

		assert.Equal(
			t,
			int32(1),
			hits.Load(),
			"concurrent callers for the same URL should coalesce into one upstream request",
		)
	})

	t.Run("transient 429 and 5xx errors fail and are not cached", func(t *testing.T) {
		t.Parallel()

		for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
			t.Run(http.StatusText(status), func(t *testing.T) {
				t.Parallel()

				var calls atomic.Int32

				c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
					if calls.Add(1) == 1 {
						return vhttp.Respond(status, nil, nil), nil
					}

					return vhttp.Respond(http.StatusOK, []byte(versionsListing), nil), nil
				})

				client := helpers.NewClient(c, nil)

				var first versionsValue

				err := client.Do(
					t.Context(),
					http.MethodGet,
					"https://registry.test/v1/providers/hashicorp/aws/versions",
					&first,
				)
				require.Error(t, err)

				var second versionsValue
				require.NoError(
					t,
					client.Do(
						t.Context(),
						http.MethodGet,
						"https://registry.test/v1/providers/hashicorp/aws/versions",
						&second,
					),
				)
				assert.Len(
					t,
					second.Versions,
					2,
					"a transient status failure must not poison subsequent lookups",
				)
			})
		}
	})
}

// versionsValue is the decoded shape of versionsListing.
type versionsValue struct {
	Versions []struct {
		Version string `json:"version"`
	} `json:"versions"`
}

// failingCloseBody is a response body whose Close fails.
type failingCloseBody struct {
	io.Reader

	err error
}

func (b failingCloseBody) Close() error {
	return b.err
}
