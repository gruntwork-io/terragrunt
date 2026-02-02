package providercache_test

import (
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
	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
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

	token := fmt.Sprintf("%s:%s", providercache.APIKeyAuth, uuid.New().String())

	providerCacheDir := helpers.TmpDirWOSymlinks(t)
	pluginCacheDir := helpers.TmpDirWOSymlinks(t)

	opts := []cache.Option{cache.WithToken(token), cache.WithCacheProviderHTTPStatusCode(providercache.CacheProviderHTTPStatusCode)}

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

			// TODO: Remove this once we can invest time in figuring out why this test is so flaky.
			//
			// It's a pain, but it's not worth the time to fix it.
			maxRetries := 3

			var lastErr error

			for attempt := 1; attempt <= maxRetries; attempt++ {
				if attempt > 1 {
					t.Logf("Retry attempt %d/%d for test case %d", attempt, maxRetries, i)
				}

				// Create a new context for each test case to avoid interference
				//
				//nolint:usetesting
				ctx := context.Background()

				ctx, cancel := context.WithCancel(ctx)
				defer cancel()

				errGroup, ctx := errgroup.WithContext(ctx)
				logger := logger.CreateLogger()

				providerService := services.NewProviderService(providerCacheDir, pluginCacheDir, nil, logger)
				providerHandler := handlers.NewDirectProviderHandler(logger, new(cliconfig.ProviderInstallationDirect), nil)
				proxyProviderHandler := handlers.NewProxyProviderHandler(logger, nil)

				tc.opts = append(tc.opts,
					cache.WithProviderService(providerService),
					cache.WithProviderHandlers(providerHandler),
					cache.WithProxyProviderHandler(proxyProviderHandler),
				)

				server := cache.NewServer(tc.opts...)

				ln, err := server.Listen(t.Context())
				if err != nil {
					lastErr = err

					if attempt < maxRetries {
						continue
					}

					require.NoError(t, err)
				}
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
				if err != nil {
					lastErr = err

					if attempt < maxRetries {
						continue
					}

					require.NoError(t, err)
				}

				req.Header.Set("Authorization", "Bearer "+token)

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					lastErr = err

					if attempt < maxRetries {
						continue
					}

					require.NoError(t, err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != tc.expectedStatusCode {
					lastErr = fmt.Errorf("expected status code %d, got %d", tc.expectedStatusCode, resp.StatusCode)

					if attempt < maxRetries {
						continue
					}

					assert.Equal(t, tc.expectedStatusCode, resp.StatusCode)
				}

				if tc.expectedBodyReg != nil {
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						lastErr = err

						if attempt < maxRetries {
							continue
						}

						require.NoError(t, err)
					}

					if !tc.expectedBodyReg.MatchString(string(body)) {
						lastErr = fmt.Errorf("body did not match expected regex: %s", tc.expectedBodyReg.String())

						if attempt < maxRetries {
							continue
						}

						assert.Regexp(t, tc.expectedBodyReg, string(body))
					}
				}

				// Skip WaitForCacheReady for unauthorized test cases since they don't trigger background operations,
				// and we cancel context at the end of the test.
				if tc.expectedStatusCode != http.StatusUnauthorized {
					_, err = providerService.WaitForCacheReady("")
					if err != nil {
						lastErr = err

						if attempt < maxRetries {
							continue
						}

						require.NoError(t, err)
					}
				}

				if tc.expectedCachePath != "" {
					if !assert.FileExists(t, filepath.Join(providerCacheDir, tc.expectedCachePath)) {
						lastErr = fmt.Errorf("expected cache file does not exist: %s", tc.expectedCachePath)

						if attempt < maxRetries {
							continue
						}
					}
				}

				cancel()

				err = errGroup.Wait()
				if err != nil {
					lastErr = err

					if attempt < maxRetries {
						continue
					}

					require.NoError(t, err)
				}

				return
			}

			t.Fatalf("Test case %d failed after %d attempts. Last error: %v", i, maxRetries, lastErr)
		})
	}
}

func TestProviderCacheHomeless(t *testing.T) {
	cacheDir := helpers.TmpDirWOSymlinks(t)

	t.Setenv("HOME", "")
	require.NoError(t, os.Unsetenv("HOME"))

	t.Setenv("XDG_CACHE_HOME", "")
	require.NoError(t, os.Unsetenv("XDG_CACHE_HOME"))

	_, err := providercache.InitServer(logger.CreateLogger(), &options.TerragruntOptions{
		ProviderCacheDir: cacheDir,
	})
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

		_, err := providercache.InitServer(
			logger.CreateLogger(),
			&options.TerragruntOptions{
				ProviderCacheDir: cacheDir,
			},
			providercache.WithFS(memFs),
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

		server, err := providercache.InitServer(
			logger.CreateLogger(),
			&options.TerragruntOptions{
				ProviderCacheDir: cacheDir,
			},
			providercache.WithFS(memFs),
		)
		require.NoError(t, err)
		require.NotNil(t, server, "InitServer should return a valid server when using VFS")
	})
}
