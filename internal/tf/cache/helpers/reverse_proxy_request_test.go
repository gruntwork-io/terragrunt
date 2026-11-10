package helpers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReverseProxyNewRequestForwardsToTarget(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		env               map[string]string
		name              string
		wantAuthorization string
	}{
		{
			name:              "credentials for the target host are added",
			env:               map[string]string{"TF_TOKEN_registry_test": "registry-token"},
			wantAuthorization: "Bearer registry-token",
		},
		{
			name: "no credentials for the target host",
			env:  map[string]string{"TF_TOKEN_other_test": "other-token"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var upstream *http.Request

			c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
				upstream = req

				return vhttp.Respond(http.StatusOK, []byte("upstream body"), nil), nil
			})

			proxy := &helpers.ReverseProxy{
				CredsSource: (&cliconfig.Config{}).CredentialsSource(tc.env),
				Transport:   c.Transport,
				Logger:      logger.CreateLogger(),
				Rewrite: func(req *httputil.ProxyRequest) {
					req.Out.Header.Set("X-Rewritten", "yes")
				},
			}

			modified := proxy.WithModifyResponse(func(resp *http.Response) error {
				resp.Header.Set("X-Modified", "yes")

				return nil
			})
			assert.Nil(t, proxy.ModifyResponse, "the original proxy should keep its own response hook")

			target := &url.URL{Scheme: "https", Host: "registry.test", Path: "/v1/providers/hashicorp/aws/versions"}
			rec := httptest.NewRecorder()

			require.NoError(t, modified.NewRequest(
				router.NewResponseWriter(rec),
				httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/proxy", nil),
				target,
			))

			require.NotNil(t, upstream, "the request should reach the target")
			assert.Equal(t, target.String(), upstream.URL.String())
			assert.Equal(t, "registry.test", upstream.Host)
			assert.Equal(t, tc.wantAuthorization, upstream.Header.Get("Authorization"))
			assert.Equal(t, "yes", upstream.Header.Get("X-Rewritten"), "the rewrite hook should run")

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "upstream body", rec.Body.String())
			assert.Equal(t, "yes", rec.Header().Get("X-Modified"), "the response hook should run")
		})
	}
}

func TestReverseProxyNewRequestPassesResponseThroughWithoutHook(t *testing.T) {
	t.Parallel()

	c := vhttp.NewMemClient(func(context.Context, *http.Request) (*http.Response, error) {
		return vhttp.Respond(http.StatusAccepted, []byte("upstream body"), nil), nil
	})

	proxy := &helpers.ReverseProxy{Transport: c.Transport, Logger: logger.CreateLogger()}
	rec := httptest.NewRecorder()

	require.NoError(t, proxy.NewRequest(
		router.NewResponseWriter(rec),
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/proxy", nil),
		&url.URL{Scheme: "https", Host: "registry.test", Path: "/file"},
	))

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, "upstream body", rec.Body.String())
}

func TestReverseProxyNewRequestUnreachableTarget(t *testing.T) {
	t.Parallel()

	var handled error

	proxy := &helpers.ReverseProxy{
		Transport: vhttp.NewNoNetworkClient().Transport,
		Logger:    logger.CreateLogger(),
		ErrorHandler: func(_ http.ResponseWriter, _ *http.Request, err error) {
			handled = err
		},
	}

	rec := httptest.NewRecorder()

	require.NoError(t, proxy.NewRequest(
		router.NewResponseWriter(rec),
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/proxy", nil),
		&url.URL{Scheme: "https", Host: "registry.test", Path: "/file"},
	))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "an unreachable target should be answered with a 503")
	assert.ErrorIs(t, handled, vhttp.ErrNoNetwork, "the error handler should receive the transport error")
}
