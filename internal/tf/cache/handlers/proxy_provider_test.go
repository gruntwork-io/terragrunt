package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	platformPath   = "/v1/providers/example/example/1.0.0/download/linux/amd64"
	registryURL    = "https://registry.example.com"
	localDownloads = "http://127.0.0.1:5758/downloads/segment/"
	localRegistry  = localDownloads + "https/registry.example.com"
)

func TestProxyProviderHandlerGetPlatformRewritesDownloadURLs(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		"proxy",
		handlers.NewProxyProviderHandler(logger.CreateLogger(), vhttp.NewMemClient(respondWith(http.StatusNotFound, "")), nil).
			String(),
	)

	body := runGetPlatform(t, registryBodies(`{
		"os": "linux",
		"download_url": "https://releases.example.com/example/1.0.0/example.zip",
		"shasums_url": "http://mirror.example.com:8080/example/1.0.0/SHA256SUMS?sig=1"
	}`))

	assert.Equal(t, localDownloads+"https/releases.example.com/example/1.0.0/example.zip", body["download_url"])
	assert.Equal(
		t,
		localDownloads+"http/mirror.example.com:8080/example/1.0.0/SHA256SUMS?sig=1",
		body["shasums_url"],
		"an http link should keep its scheme",
	)
	assert.Equal(t, "linux", body["os"], "fields other than the download urls should pass through")
	assert.NotContains(t, body, "shasums_signature_url", "a url the registry did not send should not be added")
}

func TestProxyProviderHandlerGetPlatformResolvesRelativeURLs(t *testing.T) {
	t.Parallel()

	const (
		local  = localDownloads + "https/"
		mirror = "https://mirror.example.com"
		links  = `{"download_url": "example.zip", "shasums_url": "/sums/SHA256SUMS?sig=1", ` +
			`"shasums_signature_url": "../SHA256SUMS.sig"}`
	)

	testCases := []struct {
		bodies map[string]string
		want   map[string]any
		name   string
	}{
		{
			name:   "providers endpoint on the registry host",
			bodies: registryBodies(links),
			want: map[string]any{
				"download_url":          localRegistry + platformPath + "/example.zip",
				"shasums_url":           localRegistry + "/sums/SHA256SUMS?sig=1",
				"shasums_signature_url": localRegistry + "/v1/providers/example/example/1.0.0/download/linux/SHA256SUMS.sig",
			},
		},
		{
			name: "providers endpoint on another host",
			bodies: map[string]string{
				registryURL + "/.well-known/terraform.json":                          `{"providers.v1":"` + mirror + `/api/providers/"}`,
				mirror + "/api/providers/example/example/1.0.0/download/linux/amd64": links,
			},
			want: map[string]any{
				"download_url":          local + "mirror.example.com/api/providers/example/example/1.0.0/download/linux/amd64/example.zip",
				"shasums_url":           local + "mirror.example.com/sums/SHA256SUMS?sig=1",
				"shasums_signature_url": local + "mirror.example.com/api/providers/example/example/1.0.0/download/linux/SHA256SUMS.sig",
			},
		},
		{
			name: "relative links keeping their path form",
			bodies: registryBodies(
				`{"download_url": "group%2Fproject//example.zip?redirect=https://mirror.example.com/example.zip", ` +
					`"shasums_url": "files/", "shasums_signature_url": "./sig/../SHA256SUMS.sig"}`,
			),
			want: map[string]any{
				"download_url": localRegistry + platformPath +
					"/group%252Fproject/%2Fexample.zip?redirect=https://mirror.example.com/example.zip",
				"shasums_url":           localRegistry + platformPath + "/files/",
				"shasums_signature_url": localRegistry + platformPath + "/SHA256SUMS.sig",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, runGetPlatform(t, tc.bodies))
		})
	}
}

func TestProxyProviderHandlerGetPlatformDecodesLinksAsJSON(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		want map[string]any
		name string
		body string
	}{
		{
			name: "escaped slashes",
			body: `{"download_url": "https:\/\/registry.example.com\/example.zip"}`,
			want: map[string]any{"download_url": localRegistry + "/example.zip"},
		},
		{
			name: "null link",
			body: `{"download_url": "https://registry.example.com/example.zip", "shasums_signature_url": null}`,
			want: map[string]any{
				"download_url":          localRegistry + "/example.zip",
				"shasums_signature_url": nil,
			},
		},
		{
			name: "empty link",
			body: `{"download_url": ""}`,
			want: map[string]any{"download_url": ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, runGetPlatform(t, registryBodies(tc.body)))
		})
	}
}

func TestProxyProviderHandlerGetPlatformKeepsLinkPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		link string
		want string
	}{
		{
			name: "escaped slash",
			link: "https://registry.example.com/projects/group%2Fproject/example.zip?sig=1",
			want: localRegistry + "/projects/group%252Fproject/example.zip?sig=1",
		},
		{
			name: "repeated slashes",
			link: "https://registry.example.com/example//1.0.0///example.zip?sig=1",
			want: localRegistry + "/example/%2F1.0.0/%2F/example.zip?sig=1",
		},
		{
			name: "trailing slash",
			link: "https://registry.example.com/example/1.0.0/",
			want: localRegistry + "/example/1.0.0/",
		},
		{
			name: "user info",
			link: "https://user:token@registry.example.com/example.zip",
			want: "http://user:token@127.0.0.1:5758/downloads/segment/https/registry.example.com/example.zip",
		},
		{
			name: "dot segments",
			link: "https://registry.example.com/example/./1.0.0/../example.zip",
			want: localRegistry + "/example/%2E/1.0.0/%2E%2E/example.zip",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := runGetPlatform(t, registryBodies(`{"download_url": "`+tc.link+`"}`))
			assert.Equal(t, tc.want, body["download_url"])
		})
	}
}

func TestProxyProviderHandlerGetPlatformUnusableLinks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		body string
	}{
		{
			name: "link that is not a string",
			body: `{"download_url": 5}`,
		},
		{
			name: "link that is not a valid url",
			body: `{"download_url": "https://bad host/example.zip"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := vhttp.NewMemClient(registryRoutes(map[string]string{
				"/.well-known/terraform.json": discoveryDocument,
				platformPath:                  tc.body,
			}))

			rec := httptest.NewRecorder()

			require.NoError(t, handlers.NewProxyProviderHandler(logger.CreateLogger(), c, nil).
				GetPlatform(router.NewResponseWriter(rec), newProxyRequest(t), mirroredProvider(), fakeDownloader{}))

			assert.Equal(
				t,
				http.StatusServiceUnavailable,
				rec.Code,
				"a response that cannot be rewritten should not reach the client",
			)
		})
	}
}

func TestProxyProviderHandlerDownload(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		downloadURL  string
		wantHost     string
		wantPath     string
		wantRawQuery string
	}{
		{
			name:         "absolute download url",
			downloadURL:  "https://releases.example.com/example/1.0.0/example.zip?sig=1",
			wantHost:     "releases.example.com",
			wantPath:     "/example/1.0.0/example.zip",
			wantRawQuery: "sig=1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var archiveRequest *url.URL

			c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
				archiveRequest = req.URL

				return vhttp.Respond(http.StatusOK, []byte("archive"), nil), nil
			})

			provider := mirroredProvider()
			provider.ResponseBody = &models.ResponseBody{DownloadURL: tc.downloadURL}

			rec := httptest.NewRecorder()

			require.NoError(t, handlers.NewProxyProviderHandler(logger.CreateLogger(), c, nil).
				Download(router.NewResponseWriter(rec), newProxyRequest(t), provider))

			require.NotNil(t, archiveRequest, "the archive should be requested upstream")
			assert.Equal(t, tc.wantHost, archiveRequest.Host)
			assert.Equal(t, tc.wantPath, filepath.ToSlash(archiveRequest.Path))
			assert.Equal(t, tc.wantRawQuery, archiveRequest.RawQuery)
			assert.Equal(t, "archive", rec.Body.String())
		})
	}
}

func TestProxyProviderHandlerFailures(t *testing.T) {
	t.Parallel()

	refused := vhttp.NewMemClient(respondWith(http.StatusForbidden, ""))

	t.Run("platform discovery refused", func(t *testing.T) {
		t.Parallel()

		err := handlers.NewProxyProviderHandler(logger.CreateLogger(), refused, nil).
			GetPlatform(router.NewResponseWriter(httptest.NewRecorder()), newProxyRequest(t), mirroredProvider(), fakeDownloader{})

		require.Error(t, err)
	})

	t.Run("download url that is not a valid url", func(t *testing.T) {
		t.Parallel()

		provider := mirroredProvider()
		provider.ResponseBody = &models.ResponseBody{DownloadURL: "https://bad host/example.zip"}

		err := handlers.NewProxyProviderHandler(logger.CreateLogger(), refused, nil).
			Download(router.NewResponseWriter(httptest.NewRecorder()), newProxyRequest(t), provider)

		var urlErr *url.Error

		require.ErrorAs(t, err, &urlErr)
	})
}

// fakeDownloader stands in for the downloader controller the links are rewritten to.
type fakeDownloader struct{}

func (fakeDownloader) Register(*router.Router) {}

func (fakeDownloader) URL() *url.URL {
	return &url.URL{Scheme: "http", Host: "127.0.0.1:5758", Path: "/downloads/segment"}
}

// newProxyRequest returns an inbound request for the proxy handlers.
func newProxyRequest(t *testing.T) *http.Request {
	t.Helper()

	return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/proxy", nil)
}

// registryBodies returns what registry.example.com serves when its platform endpoint answers with platform.
func registryBodies(platform string) map[string]string {
	return map[string]string{
		registryURL + "/.well-known/terraform.json": discoveryDocument,
		registryURL + platformPath:                  platform,
	}
}

// runGetPlatform runs GetPlatform against a registry serving bodies by URL and returns the rewritten response.
func runGetPlatform(t *testing.T, bodies map[string]string) map[string]any {
	t.Helper()

	c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		body, ok := bodies[req.URL.String()]
		if !ok {
			return vhttp.Respond(http.StatusNotFound, nil, nil), nil
		}

		return vhttp.Respond(http.StatusOK, []byte(body), http.Header{"Content-Type": []string{"application/json"}}), nil
	})

	rec := httptest.NewRecorder()

	require.NoError(t, handlers.NewProxyProviderHandler(logger.CreateLogger(), c, nil).
		GetPlatform(router.NewResponseWriter(rec), newProxyRequest(t), mirroredProvider(), fakeDownloader{}))
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	return body
}
