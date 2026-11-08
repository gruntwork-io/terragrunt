package controllers_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/controllers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/middleware"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testToken        = "cache-server-token"
	testRegistry     = "registry.example.com"
	testServerAddr   = "127.0.0.1:5758"
	testDiscoveryDoc = `{"modules.v1":"/v1/modules/","providers.v1":"/v1/providers/"}`
	testArchiveName  = "terraform-provider-example_1.0.0_linux_amd64.zip"
	testArchiveURL   = "https://releases.example.com/example/1.0.0/" + testArchiveName
)

func TestDiscoveryControllerListsEndpoints(t *testing.T) {
	t.Parallel()

	providerController := &controllers.ProviderController{Logger: logger.CreateLogger()}
	moduleController := &controllers.ModuleController{Logger: logger.CreateLogger()}

	root := router.New()
	root.Register(&controllers.DiscoveryController{
		Endpointers: []controllers.Endpointer{providerController, moduleController},
	})
	root.Group("v1").Register(providerController, moduleController)

	rec := serve(t, root, "/.well-known/terraform.json", "")

	require.Equal(t, http.StatusOK, rec.Code)

	var endpoints map[string]string

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &endpoints))
	assert.Equal(t, map[string]string{
		"modules.v1":   "/v1/modules/",
		"providers.v1": "/v1/providers",
	}, endpoints, "discovery should merge the endpoints of every controller")
}

func TestModuleControllerProxy(t *testing.T) {
	t.Parallel()

	const target = "/v1/modules/" + testRegistry + "/acme/vpc/aws/versions?page=2"

	testCases := []struct {
		auth              router.MiddlewareFunc
		name              string
		target            string
		authorization     string
		wantUpstreamURL   string
		wantUpstreamToken string
		wantStatus        int
	}{
		{
			name:              "authorized request is forwarded with upstream credentials",
			auth:              middleware.KeyAuth(testToken),
			target:            target,
			authorization:     "Bearer " + testToken,
			wantStatus:        http.StatusOK,
			wantUpstreamURL:   "https://" + testRegistry + "/v1/modules/acme/vpc/aws/versions?page=2",
			wantUpstreamToken: "Bearer upstream-token",
		},
		{
			name:       "request without the cache server token is rejected",
			auth:       middleware.KeyAuth(testToken),
			target:     target,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:              "controller without auth middleware forwards the request",
			target:            target,
			wantStatus:        http.StatusOK,
			wantUpstreamURL:   "https://" + testRegistry + "/v1/modules/acme/vpc/aws/versions?page=2",
			wantUpstreamToken: "Bearer upstream-token",
		},
		{
			name:          "registry that is not configured is not proxied",
			auth:          middleware.KeyAuth(testToken),
			target:        "/v1/modules/other.example.com/acme/vpc/aws/versions",
			authorization: "Bearer " + testToken,
			wantStatus:    http.StatusNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			up := newUpstream(map[string][]byte{
				"https://" + testRegistry + "/.well-known/terraform.json":       []byte(testDiscoveryDoc),
				"https://" + testRegistry + "/v1/modules/acme/vpc/aws/versions": []byte(`{"modules":[]}`),
			})

			l := logger.CreateLogger()
			creds := (&cliconfig.Config{}).CredentialsSource(map[string]string{
				"TF_TOKEN_registry_example_com": "upstream-token",
			})
			discoverer := handlers.NewCommonProviderHandler(l, up.client(), nil, nil)

			root := router.New()
			root.Group("v1").Register(&controllers.ModuleController{
				AuthMiddleware: tc.auth,
				ProxyModuleHandler: handlers.NewProxyModuleHandler(
					l, up.client(), creds, discoverer, []string{testRegistry},
				),
				Logger: l,
			})

			rec := serve(t, root, tc.target, tc.authorization)

			assert.Equal(t, tc.wantStatus, rec.Code)

			proxied := up.requestsTo("/v1/modules/")
			if tc.wantUpstreamURL == "" {
				assert.Empty(t, proxied, "a rejected request should never reach the upstream registry")

				return
			}

			require.Len(t, proxied, 1)
			assert.Equal(t, tc.wantUpstreamURL, proxied[0].url)
			assert.Equal(
				t,
				tc.wantUpstreamToken,
				proxied[0].authorization,
				"upstream should get the user credentials, not the cache server token",
			)
			assert.JSONEq(t, `{"modules":[]}`, rec.Body.String())
		})
	}
}

func TestProviderControllerGetVersionsMergesHandlers(t *testing.T) {
	t.Parallel()

	failing := &fakeProviderHandler{canHandle: true, err: errors.New("registry unavailable")}
	skipped := &fakeProviderHandler{versions: models.Versions{{Version: "9.9.9"}}}
	serving := &fakeProviderHandler{
		canHandle: true,
		versions: models.Versions{
			{Version: "1.0.0", Protocols: []string{"5.0"}},
			{Version: "v2.0.0"},
		},
	}
	alsoServing := &fakeProviderHandler{
		canHandle: true,
		versions:  models.Versions{{Version: "1.1.0", Protocols: []string{"5.0"}}},
	}

	root := router.New()
	root.Group("v1").Register(&controllers.ProviderController{
		Logger:           logger.CreateLogger(),
		ProviderHandlers: []handlers.ProviderHandler{failing, skipped, serving, alsoServing},
	})

	rec := serve(t, root, "/v1/providers/"+testRegistry+"/example/example/versions", "")

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		ID       string          `json:"id"`
		Versions models.Versions `json:"versions"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, testRegistry+"/example/example", body.ID)

	versions := make([]string, 0, len(body.Versions))
	for _, v := range body.Versions {
		versions = append(versions, v.Version)
	}

	assert.Equal(
		t,
		[]string{"1.0.0", "1.1.0"},
		versions,
		"only valid versions from handlers that can serve the provider should be listed",
	)
}

func TestProviderControllerGetPlatformRewritesDownloadURLs(t *testing.T) {
	t.Parallel()

	const platformPath = "/v1/providers/example/example/1.0.0/download/linux/amd64"

	up := newUpstream(map[string][]byte{
		"https://" + testRegistry + "/.well-known/terraform.json": []byte(testDiscoveryDoc),
		"https://" + testRegistry + platformPath: []byte(`{
			"os": "linux",
			"download_url": "` + testArchiveURL + `",
			"shasums_url": "https://releases.example.com/example/1.0.0/SHA256SUMS"
		}`),
	})

	l := logger.CreateLogger()
	proxy := handlers.NewProxyProviderHandler(l, up.client(), nil)
	downloader := controllers.NewDownloaderController(nil, proxy)

	root := router.New()
	root.Server.Addr = testServerAddr
	root.Register(downloader)
	root.Group("v1").Register(&controllers.ProviderController{
		Logger:               l,
		AuthMiddleware:       middleware.KeyAuth(testToken),
		DownloaderController: downloader,
		ProxyProviderHandler: proxy,
	})

	rec := serve(t, root, "/v1/providers/"+testRegistry+"/example/example/1.0.0/download/linux/amd64", "Bearer "+testToken)

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	downloadBase := "http://" + testServerAddr + "/downloads/" + downloader.Segment() +
		"/https/releases.example.com/example/1.0.0/"
	assert.Equal(t, downloadBase+testArchiveName, body["download_url"], "download url should point at the cache server")
	assert.Equal(t, downloadBase+"SHA256SUMS", body["shasums_url"], "shasums url should point at the cache server")
	assert.Equal(t, "linux", body["os"], "other fields should pass through")
	assert.Len(t, up.requestsTo(platformPath), 1)
}

func TestProviderControllerCachesProviderForDownload(t *testing.T) {
	t.Parallel()

	const requestID = "cache-request"

	archive := providerArchive(t)
	up := newUpstream(map[string][]byte{testArchiveURL: archive})
	l := logger.CreateLogger()

	// The provider service locks every package with an OS file lock, so it needs the real filesystem.
	tempDir := t.TempDir()
	require.NoError(t, os.Chmod(tempDir, 0o700))

	service := services.NewProviderService(
		helpers.TmpDirWOSymlinks(t),
		"",
		nil,
		l,
		venvtest.NewOSWithEmptyEnv().WithHTTP(up.client()).WithTempDir(func() string { return tempDir }),
	)
	require.NoError(t, service.Init())

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	runErr := make(chan error, 1)

	go func() { runErr <- service.Run(ctx) }()

	failing := &fakeProviderHandler{canHandle: true, err: errors.New("registry unavailable")}
	serving := &fakeProviderHandler{
		canHandle: true,
		platform: &models.ResponseBody{
			Filename:    testArchiveName,
			DownloadURL: testArchiveURL,
			Origin:      models.OriginMirror,
		},
	}

	downloader := controllers.NewDownloaderController(service, handlers.NewProxyProviderHandler(l, up.client(), nil))

	root := router.New()
	root.Register(downloader)
	root.Group("v1").Register(&controllers.ProviderController{
		Logger:                      l,
		DownloaderController:        downloader,
		ProviderService:             service,
		ProviderHandlers:            []handlers.ProviderHandler{failing, serving},
		CacheProviderHTTPStatusCode: http.StatusLocked,
	})

	rec := serve(t, root, "/v1/providers/"+requestID+"/"+testRegistry+"/example/example/1.0.0/download/linux/amd64", "")
	assert.Equal(t, http.StatusLocked, rec.Code, "a cache request should be answered with the configured status")

	cached, err := service.WaitForCacheReady(requestID)
	require.NoError(t, err)
	require.Len(t, cached, 1, "the requested provider should be cached")

	rec = serve(
		t,
		root,
		"/downloads/"+downloader.Segment()+"/https/releases.example.com/example/1.0.0/"+testArchiveName,
		"",
	)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, archive, rec.Body.Bytes(), "download should serve the cached archive")
	assert.Len(t, up.requestsTo("/example/1.0.0/"), 1, "the archive should be fetched upstream only once")

	cancel()
	require.NoError(t, <-runErr)
}

func TestDownloaderControllerProxiesUncachedDownload(t *testing.T) {
	t.Parallel()

	httpArchiveURL := "http://releases.example.com/example/1.0.0/" + testArchiveName

	testCases := []struct {
		name            string
		scheme          string
		segment         func(*controllers.DownloaderController) string
		wantUpstreamURL string
		wantStatus      int
	}{
		{
			name:            "download through the secret segment",
			scheme:          "https",
			segment:         (*controllers.DownloaderController).Segment,
			wantStatus:      http.StatusOK,
			wantUpstreamURL: testArchiveURL + "?X-Amz-Signature=abc",
		},
		{
			name:            "http download keeps its scheme",
			scheme:          "http",
			segment:         (*controllers.DownloaderController).Segment,
			wantStatus:      http.StatusOK,
			wantUpstreamURL: httpArchiveURL + "?X-Amz-Signature=abc",
		},
		{
			name:       "download with an unsupported scheme",
			scheme:     "ftp",
			segment:    (*controllers.DownloaderController).Segment,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "download through another segment",
			scheme:     "https",
			segment:    func(*controllers.DownloaderController) string { return "guessed" },
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			up := newUpstream(map[string][]byte{
				testArchiveURL: []byte("archive bytes"),
				httpArchiveURL: []byte("archive bytes"),
			})
			l := logger.CreateLogger()

			downloader := controllers.NewDownloaderController(
				services.NewProviderService("", "", nil, l, venvtest.New()),
				handlers.NewProxyProviderHandler(l, up.client(), nil),
			)

			root := router.New()
			root.Register(downloader)

			rec := serve(
				t,
				root,
				"/downloads/"+tc.segment(
					downloader,
				)+"/"+tc.scheme+"/releases.example.com/example/1.0.0/"+testArchiveName+"?X-Amz-Signature=abc",
				"",
			)

			assert.Equal(t, tc.wantStatus, rec.Code)

			proxied := up.requestsTo("/example/1.0.0/")
			if tc.wantUpstreamURL == "" {
				assert.Empty(t, proxied, "a request the route rejects should never reach upstream")

				return
			}

			require.Len(t, proxied, 1)
			assert.Equal(t, tc.wantUpstreamURL, proxied[0].url, "query string should be forwarded so signed urls keep working")
			assert.Equal(t, "archive bytes", rec.Body.String())
		})
	}
}

func TestDownloaderControllerSegmentIsUniquePerController(t *testing.T) {
	t.Parallel()

	first := controllers.NewDownloaderController(nil, nil)
	second := controllers.NewDownloaderController(nil, nil)

	require.NotEmpty(t, first.Segment())
	assert.NotEqual(t, first.Segment(), second.Segment(), "every controller should get its own secret segment")
}

// fakeProviderHandler answers provider requests with fixed values.
type fakeProviderHandler struct {
	err       error
	platform  *models.ResponseBody
	versions  models.Versions
	canHandle bool
}

func (h *fakeProviderHandler) CanHandleProvider(*models.Provider) bool {
	return h.canHandle
}

func (h *fakeProviderHandler) GetVersions(context.Context, *models.Provider) (models.Versions, error) {
	return h.versions, h.err
}

func (h *fakeProviderHandler) GetPlatform(context.Context, *models.Provider) (*models.ResponseBody, error) {
	return h.platform, h.err
}

func (h *fakeProviderHandler) DiscoveryURL(context.Context, string) (*handlers.RegistryURLs, error) {
	return handlers.DefaultRegistryURLs, nil
}

// upstreamRequest is what the fake upstream received.
type upstreamRequest struct {
	url           string
	authorization string
}

// upstream is a fake registry that serves fixed bodies keyed by URL without query.
type upstream struct {
	bodies   map[string][]byte
	requests []upstreamRequest
	mu       sync.Mutex
}

func newUpstream(bodies map[string][]byte) *upstream {
	return &upstream{bodies: bodies}
}

func (u *upstream) client() vhttp.Client {
	return vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		u.mu.Lock()
		u.requests = append(u.requests, upstreamRequest{
			url:           req.URL.String(),
			authorization: req.Header.Get("Authorization"),
		})
		u.mu.Unlock()

		key := req.URL.Scheme + "://" + req.URL.Host + req.URL.Path

		body, ok := u.bodies[key]
		if !ok {
			return vhttp.Respond(http.StatusNotFound, nil, nil), nil
		}

		return vhttp.Respond(http.StatusOK, body, http.Header{"Content-Type": []string{"application/json"}}), nil
	})
}

// requestsTo returns the requests whose URL path contains pathPart.
func (u *upstream) requestsTo(pathPart string) []upstreamRequest {
	u.mu.Lock()
	defer u.mu.Unlock()

	return slices.DeleteFunc(slices.Clone(u.requests), func(req upstreamRequest) bool {
		return !strings.Contains(req.url, pathPart)
	})
}

// serve sends a GET for target through h and returns the recorded response.
func serve(t *testing.T, h http.Handler, target, authorization string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

// providerArchive builds a zip archive standing in for a provider release.
func providerArchive(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	w, err := zw.Create("terraform-provider-example_v1.0.0")
	require.NoError(t, err)

	_, err = io.Copy(w, strings.NewReader("provider binary"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return buf.Bytes()
}
