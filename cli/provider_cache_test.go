package cli_test

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
	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/cache"
	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/gruntwork-io/terragrunt/terraform/cliconfig"
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

	token := fmt.Sprintf("%s:%s", cli.APIKeyAuth, uuid.New().String())

	providerCacheDir := t.TempDir()
	pluginCacheDir := t.TempDir()

	opts := []cache.Option{cache.WithToken(token)}

	testCases := []struct {
		opts               []cache.Option
		fullURLPath        string
		relURLPath         string
		expectedStatusCode int
		expectedBodyReg    *regexp.Regexp
		expectedCachePath  string
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
	//
	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			errGroup, ctx := errgroup.WithContext(ctx)

			providerService := services.NewProviderService(providerCacheDir, pluginCacheDir, nil, log.New())
			providerHandler := handlers.NewProviderDirectHandler(providerService, cli.CacheProviderHTTPStatusCode, new(cliconfig.ProviderInstallationDirect), nil)

			testCase.opts = append(testCase.opts, cache.WithServices(providerService), cache.WithProviderHandlers(providerHandler))

			server := cache.NewServer(testCase.opts...)
			ln, err := server.Listen()
			require.NoError(t, err)
			defer ln.Close()

			errGroup.Go(func() error {
				return server.Run(ctx, ln)
			})

			urlPath := server.ProviderController.URL()
			urlPath.Path += testCase.relURLPath

			if testCase.fullURLPath != "" {
				urlPath.Path = testCase.fullURLPath
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath.String(), nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, testCase.expectedStatusCode, resp.StatusCode)

			if testCase.expectedBodyReg != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Regexp(t, testCase.expectedBodyReg, string(body))
			}

			_, err = providerService.WaitForCacheReady("")
			require.NoError(t, err)

			if testCase.expectedCachePath != "" {
				assert.FileExists(t, filepath.Join(providerCacheDir, testCase.expectedCachePath))
			}

			cancel()
			err = errGroup.Wait()
			require.NoError(t, err)
		})
	}
}

func TestProviderCacheWithProviderCacheDir(t *testing.T) {
	// testing.T can Setenv, but can't Unsetenv
	unsetEnv := func(t *testing.T, v string) {
		t.Helper()

		// let testing.T do the recovery and work around t.Parallel()
		t.Setenv(v, "")
		require.NoError(t, os.Unsetenv(v))
	}

	t.Run("Homeless", func(t *testing.T) { //nolint:paralleltest
		cacheDir := t.TempDir()

		unsetEnv(t, "HOME")
		unsetEnv(t, "XDG_CACHE_HOME")

		_, err := cli.InitProviderCacheServer(&options.TerragruntOptions{
			ProviderCacheDir: cacheDir,
		})
		require.NoError(t, err, "ProviderCache shouldn't read HOME environment variable")
	})

	t.Run("NoNewDirectoriesAtHOME", func(t *testing.T) {
		home := t.TempDir()
		cacheDir := t.TempDir()

		t.Setenv("HOME", home)

		_, err := cli.InitProviderCacheServer(&options.TerragruntOptions{
			ProviderCacheDir: cacheDir,
		})
		require.NoError(t, err)

		// Cache server shouldn't create any directory at $HOME when ProviderCacheDir is specified
		entries, err := os.ReadDir(home)
		require.NoError(t, err)
		require.Empty(t, entries, "No new directories should be created at $HOME")
	})
}
