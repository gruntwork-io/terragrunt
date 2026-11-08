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

const platformPath = "/v1/providers/example/example/1.0.0/download/linux/amd64"

func TestProxyProviderHandlerGetPlatformRewritesDownloadURLs(t *testing.T) {
	t.Parallel()

	c := vhttp.NewMemClient(registryRoutes(map[string]string{
		"/.well-known/terraform.json": discoveryDocument,
		platformPath: `{
			"os": "linux",
			"download_url": "https://releases.example.com/example/1.0.0/example.zip",
			"shasums_url": "http://mirror.example.com:8080/example/1.0.0/SHA256SUMS?sig=1"
		}`,
	}))

	handler := handlers.NewProxyProviderHandler(logger.CreateLogger(), c, nil)
	assert.Equal(t, "proxy", handler.String())

	rec := httptest.NewRecorder()

	require.NoError(
		t,
		handler.GetPlatform(router.NewResponseWriter(rec), newProxyRequest(t), mirroredProvider(), fakeDownloader{}),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.Equal(
		t,
		"http://127.0.0.1:5758/downloads/segment/https/releases.example.com/example/1.0.0/example.zip",
		body["download_url"],
	)
	assert.Equal(
		t,
		"http://127.0.0.1:5758/downloads/segment/http/mirror.example.com:8080/example/1.0.0/SHA256SUMS?sig=1",
		body["shasums_url"],
		"an http link should keep its scheme",
	)
	assert.Equal(t, "linux", body["os"], "fields other than the download urls should pass through")
	assert.NotContains(t, body, "shasums_signature_url", "a url the registry did not send should not be added")
}

// TestProxyProviderHandlerGetPlatformResolvesRelativeURLs pins that relative links resolve as cached provider links do.
func TestProxyProviderHandlerGetPlatformResolvesRelativeURLs(t *testing.T) {
	t.Parallel()

	const local = "http://127.0.0.1:5758/downloads/segment/https/"

	const links = `{"download_url": "example.zip", "shasums_url": "/sums/SHA256SUMS?sig=1", ` +
		`"shasums_signature_url": "../SHA256SUMS.sig"}`

	testCases := []struct {
		want         map[string]string
		name         string
		discovery    string
		platformPath string
		links        string
	}{
		{
			name:         "providers endpoint on the registry host",
			discovery:    discoveryDocument,
			platformPath: platformPath,
			links:        links,
			want: map[string]string{
				"download_url":          local + "registry.example.com" + platformPath + "/example.zip",
				"shasums_url":           local + "registry.example.com/sums/SHA256SUMS?sig=1",
				"shasums_signature_url": local + "registry.example.com/v1/providers/example/example/1.0.0/download/linux/SHA256SUMS.sig",
			},
		},
		{
			name:         "providers endpoint on another host",
			discovery:    `{"providers.v1":"https://mirror.example.com/api/providers/"}`,
			platformPath: "/api/providers/example/example/1.0.0/download/linux/amd64",
			links:        links,
			want: map[string]string{
				"download_url":          local + "mirror.example.com/api/providers/example/example/1.0.0/download/linux/amd64/example.zip",
				"shasums_url":           local + "mirror.example.com/sums/SHA256SUMS?sig=1",
				"shasums_signature_url": local + "mirror.example.com/api/providers/example/example/1.0.0/download/linux/SHA256SUMS.sig",
			},
		},
		{
			name:         "relative links keeping their path form",
			discovery:    discoveryDocument,
			platformPath: platformPath,
			links: `{"download_url": "group%2Fproject//example.zip?redirect=https://mirror.example.com/example.zip", ` +
				`"shasums_url": "files/", "shasums_signature_url": "./sig/../SHA256SUMS.sig"}`,
			want: map[string]string{
				"download_url": local + "registry.example.com" + platformPath +
					"/group%252Fproject/%2Fexample.zip?redirect=https://mirror.example.com/example.zip",
				"shasums_url":           local + "registry.example.com" + platformPath + "/files/",
				"shasums_signature_url": local + "registry.example.com" + platformPath + "/SHA256SUMS.sig",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := vhttp.NewMemClient(registryRoutes(map[string]string{
				"/.well-known/terraform.json": tc.discovery,
				tc.platformPath:               tc.links,
			}))

			rec := httptest.NewRecorder()

			require.NoError(t, handlers.NewProxyProviderHandler(logger.CreateLogger(), c, nil).
				GetPlatform(router.NewResponseWriter(rec), newProxyRequest(t), mirroredProvider(), fakeDownloader{}))
			require.Equal(t, http.StatusOK, rec.Code)

			var body map[string]string

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tc.want, body)
		})
	}
}

// TestProxyProviderHandlerGetPlatformDecodesLinksAsJSON pins that links are decoded as JSON strings and that null or empty links pass through.
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
			want: map[string]any{
				"download_url": "http://127.0.0.1:5758/downloads/segment/https/registry.example.com/example.zip",
			},
		},
		{
			name: "null link",
			body: `{"download_url": "https://registry.example.com/example.zip", "shasums_signature_url": null}`,
			want: map[string]any{
				"download_url":          "http://127.0.0.1:5758/downloads/segment/https/registry.example.com/example.zip",
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

			c := vhttp.NewMemClient(registryRoutes(map[string]string{
				"/.well-known/terraform.json": discoveryDocument,
				platformPath:                  tc.body,
			}))

			rec := httptest.NewRecorder()

			require.NoError(t, handlers.NewProxyProviderHandler(logger.CreateLogger(), c, nil).
				GetPlatform(router.NewResponseWriter(rec), newProxyRequest(t), mirroredProvider(), fakeDownloader{}))
			require.Equal(t, http.StatusOK, rec.Code)

			var body map[string]any

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tc.want, body)
		})
	}
}

// TestProxyProviderHandlerGetPlatformKeepsLinkPath pins that a rewritten link keeps its slashes and dot segments routable.
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
			want: "http://127.0.0.1:5758/downloads/segment/https/registry.example.com/projects/group%252Fproject/example.zip?sig=1",
		},
		{
			name: "repeated slashes",
			link: "https://registry.example.com/example//1.0.0///example.zip?sig=1",
			want: "http://127.0.0.1:5758/downloads/segment/https/registry.example.com/example/%2F1.0.0/%2F/example.zip?sig=1",
		},
		{
			name: "trailing slash",
			link: "https://registry.example.com/example/1.0.0/",
			want: "http://127.0.0.1:5758/downloads/segment/https/registry.example.com/example/1.0.0/",
		},
		{
			name: "dot segments",
			link: "https://registry.example.com/example/./1.0.0/../example.zip",
			want: "http://127.0.0.1:5758/downloads/segment/https/registry.example.com/example/%2E/1.0.0/%2E%2E/example.zip",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := vhttp.NewMemClient(registryRoutes(map[string]string{
				"/.well-known/terraform.json": discoveryDocument,
				platformPath:                  `{"download_url": "` + tc.link + `"}`,
			}))

			rec := httptest.NewRecorder()

			require.NoError(t, handlers.NewProxyProviderHandler(logger.CreateLogger(), c, nil).
				GetPlatform(router.NewResponseWriter(rec), newProxyRequest(t), mirroredProvider(), fakeDownloader{}))
			require.Equal(t, http.StatusOK, rec.Code)

			var body map[string]string

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
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

			c := vhttp.NewMemClient(func(ctx context.Context, req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/.well-known/terraform.json" {
					return registryRoutes(map[string]string{req.URL.Path: discoveryDocument})(ctx, req)
				}

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
