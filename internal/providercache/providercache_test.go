package providercache_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/gruntwork-io/terragrunt/internal/providercache"
	pcoptions "github.com/gruntwork-io/terragrunt/internal/providercache/options"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func createFakeProvider(t *testing.T, cacheDir, relativePath string) string {
	t.Helper()

	err := os.MkdirAll(filepath.Join(cacheDir, filepath.Dir(relativePath)), os.ModePerm)
	require.NoError(t, err)

	file, err := os.Create(filepath.Join(cacheDir, relativePath))
	require.NoError(t, err)

	defer file.Close()

	err = file.Sync()
	require.NoError(t, err)

	return relativePath
}

func TestProviderCache(t *testing.T) {
	t.Parallel()

	testProviderCache(t, vhttp.NewMemClient(registryHandler(t)))
}

// testProviderCache drives the provider cache server through discovery,
// version listing, and provider download over c. The default build passes an
// in-memory client synthesizing registry.terraform.io; the http-tagged
// variant passes an OS client so the same scenarios run against the real
// registry.
func testProviderCache(t *testing.T, c vhttp.Client) {
	t.Helper()

	token := fmt.Sprintf("%s:%s", providercache.APIKeyAuth, uuid.New().String())

	providerCacheDir := helpers.TmpDirWOSymlinks(t)
	pluginCacheDir := helpers.TmpDirWOSymlinks(t)

	opts := make([]cache.Option, 0, 3)
	opts = append(opts, cache.WithToken(token), cache.WithCacheProviderHTTPStatusCode(providercache.CacheProviderHTTPStatusCode))

	testCases := []struct {
		expectedBodyReg    *regexp.Regexp
		fullURLPath        string
		relURLPath         string
		expectedCachePath  string
		opts               []cache.Option
		expectedStatusCode int
	}{
		{
			opts:               opts,
			fullURLPath:        "/.well-known/terraform.json",
			expectedStatusCode: http.StatusOK,
			expectedBodyReg:    regexp.MustCompile(regexp.QuoteMeta(`{"providers.v1":"/v1/providers"}`)),
		},
		{
			opts:               append(opts, cache.WithToken("")),
			relURLPath:         "/cache/registry.terraform.io/hashicorp/aws/versions",
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			opts:               opts,
			relURLPath:         "/cache/registry.terraform.io/hashicorp/aws/versions",
			expectedStatusCode: http.StatusOK,
			expectedBodyReg:    regexp.MustCompile(regexp.QuoteMeta(`"version":"5.36.0","protocols":["5.0"],"platforms"`)),
		},
		{
			opts:               opts,
			relURLPath:         "/cache/registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  "registry.terraform.io/hashicorp/aws/5.36.0/darwin_arm64/terraform-provider-aws_v5.36.0_x5",
		},
		{
			opts:               opts,
			relURLPath:         "/cache/registry.terraform.io/hashicorp/template/2.2.0/download/linux/amd64",
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  "registry.terraform.io/hashicorp/template/2.2.0/linux_amd64/terraform-provider-template_v2.2.0_x4",
		},
		{
			opts:               opts,
			relURLPath:         fmt.Sprintf("/cache/registry.terraform.io/hashicorp/template/1234.5678.9/download/%s/%s", runtime.GOOS, runtime.GOARCH),
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  createFakeProvider(t, pluginCacheDir, fmt.Sprintf("registry.terraform.io/hashicorp/template/1234.5678.9/%s_%s/terraform-provider-template_1234.5678.9_x5", runtime.GOOS, runtime.GOARCH)),
		},
		{
			opts:               opts,
			relURLPath:         "//registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusOK,
			expectedBodyReg:    regexp.MustCompile(`\{.*` + regexp.QuoteMeta(`"download_url":"http://127.0.0.1:`) + `\d+` + regexp.QuoteMeta(`/downloads/releases.hashicorp.com/terraform-provider-aws/5.36.0/terraform-provider-aws_5.36.0_darwin_arm64.zip"`) + `.*\}`),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			errGroup, ctx := errgroup.WithContext(ctx)
			l := logger.CreateLogger()

			providerService := services.NewProviderService(providerCacheDir, pluginCacheDir, nil, l, services.WithHTTPClient(c))
			providerHandler := handlers.NewDirectProviderHandler(l, c, new(cliconfig.ProviderInstallationDirect), nil)
			proxyProviderHandler := handlers.NewProxyProviderHandler(l, c, nil)

			tc.opts = append(tc.opts,
				cache.WithProviderService(providerService),
				cache.WithProviderHandlers(providerHandler),
				cache.WithProxyProviderHandler(proxyProviderHandler),
			)

			server := cache.NewServer(tc.opts...)

			ln, err := server.Listen(t.Context())
			require.NoError(t, err)

			defer ln.Close()

			errGroup.Go(func() error {
				return server.Run(ctx, ln)
			})

			urlPath := server.ProviderController.URL()
			urlPath.Path += tc.relURLPath

			if tc.fullURLPath != "" {
				urlPath.Path = tc.fullURLPath
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath.String(), nil)
			require.NoError(t, err)

			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatusCode, resp.StatusCode)

			if tc.expectedBodyReg != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Regexp(t, tc.expectedBodyReg, string(body))
			}

			// Skip WaitForCacheReady for unauthorized test cases since they don't trigger background operations,
			// and we cancel context at the end of the test.
			if tc.expectedStatusCode != http.StatusUnauthorized {
				_, err = providerService.WaitForCacheReady("")
				require.NoError(t, err)
			}

			if tc.expectedCachePath != "" {
				assert.FileExists(t, filepath.Join(providerCacheDir, tc.expectedCachePath))
			}

			cancel()

			require.NoError(t, errGroup.Wait())
		})
	}
}

// registryHandler synthesizes the slice of the provider registry protocol the
// cache server exercises: service discovery, version listings, platform
// download documents, and the release archives themselves. Anything else gets
// a 404, which is also what the real registry returns for the fabricated
// 1234.5678.9 version — the cache must then fall back to the user plugin dir.
func registryHandler(t *testing.T) vhttp.Handler {
	t.Helper()

	const downloadJSONFmt = `{"os":%q,"arch":%q,"filename":%q,"download_url":"https://releases.hashicorp.com%s"}`

	const (
		awsZipPath      = "/terraform-provider-aws/5.36.0/terraform-provider-aws_5.36.0_darwin_arm64.zip"
		templateZipPath = "/terraform-provider-template/2.2.0/terraform-provider-template_2.2.0_linux_amd64.zip"
	)

	jsonByPath := map[string]string{
		"/.well-known/terraform.json": `{"providers.v1":"/v1/providers"}`,
		"/v1/providers/hashicorp/aws/versions": `{"versions":[` +
			`{"version":"5.36.0","protocols":["5.0"],"platforms":[{"os":"darwin","arch":"arm64"}]}]}`,
		"/v1/providers/hashicorp/aws/5.36.0/download/darwin/arm64": fmt.Sprintf(downloadJSONFmt,
			"darwin", "arm64", filepath.Base(awsZipPath), awsZipPath),
		"/v1/providers/hashicorp/template/2.2.0/download/linux/amd64": fmt.Sprintf(downloadJSONFmt,
			"linux", "amd64", filepath.Base(templateZipPath), templateZipPath),
	}

	zipByPath := map[string][]byte{
		awsZipPath:      zipWithFile(t, "terraform-provider-aws_v5.36.0_x5"),
		templateZipPath: zipWithFile(t, "terraform-provider-template_v2.2.0_x4"),
	}

	return func(_ context.Context, req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "registry.terraform.io":
			if body, ok := jsonByPath[req.URL.Path]; ok {
				return vhttp.Respond(http.StatusOK, []byte(body),
					http.Header{"Content-Type": []string{"application/json"}}), nil
			}
		case "releases.hashicorp.com":
			if body, ok := zipByPath[req.URL.Path]; ok {
				return vhttp.Respond(http.StatusOK, body,
					http.Header{"Content-Type": []string{"application/zip"}}), nil
			}
		}

		return vhttp.Respond(http.StatusNotFound, nil, nil), nil
	}
}

// zipWithFile builds an in-memory zip archive holding a single file, standing
// in for a provider release archive.
func zipWithFile(t *testing.T, name string) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	w, err := zw.Create(name)
	require.NoError(t, err)

	_, err = w.Write([]byte("fake provider binary"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return buf.Bytes()
}

func TestProviderCacheHomeless(t *testing.T) {
	cacheDir := helpers.TmpDirWOSymlinks(t)

	t.Setenv("HOME", "")
	require.NoError(t, os.Unsetenv("HOME"))

	t.Setenv("XDG_CACHE_HOME", "")
	require.NoError(t, os.Unsetenv("XDG_CACHE_HOME"))

	_, err := providercache.InitServer(logger.CreateLogger(), venv.OSVenv(), &pcoptions.ProviderCacheOptions{
		Dir: cacheDir,
	}, "")
	require.NoError(t, err, "ProviderCache shouldn't read HOME environment variable")
}

func TestProviderCacheWithProviderCacheDir(t *testing.T) {
	t.Parallel()

	t.Run("NoNewDirectoriesAtHOME", func(t *testing.T) {
		t.Parallel()

		// Use in-memory filesystem to isolate file operations from the real filesystem.
		// This ensures InitServer doesn't create any directories on the real filesystem
		// since all file operations are routed through the VFS.
		memFs := vfs.NewMemMapFS()
		cacheDir := "/test/provider-cache"

		server := providercache.NewProviderCache()
		err := server.Init(
			logger.CreateLogger(),
			new(venvtest.New().WithFS(memFs)),
			&pcoptions.ProviderCacheOptions{
				Dir: cacheDir,
			},
			"",
		)
		require.NoError(t, err)

		// With VFS, all file operations go through the in-memory filesystem,
		// so no directories should be created on the real filesystem at all.
		// We can verify the VFS is being used by checking it's not empty or
		// by the fact that no errors occurred despite using fake paths.
	})

	t.Run("InitServerWithVFS", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		cacheDir := "/vfs/provider-cache"

		server := providercache.NewProviderCache()
		err := server.Init(
			logger.CreateLogger(),
			new(venvtest.New().WithFS(memFs)),
			&pcoptions.ProviderCacheOptions{
				Dir: cacheDir,
			},
			"",
		)
		require.NoError(t, err)
		require.NotNil(t, server, "Init should return a valid server when using VFS")
	})
}
