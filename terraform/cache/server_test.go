package cache

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
	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func createFakeProvider(t *testing.T, cacheDir, relativePath string) string {
	err := os.MkdirAll(filepath.Join(cacheDir, filepath.Dir(relativePath)), os.ModePerm)
	require.NoError(t, err)

	file, err := os.Create(filepath.Join(cacheDir, relativePath))
	require.NoError(t, err)
	defer file.Close()

	err = file.Sync()
	require.NoError(t, err)

	return relativePath
}

func TestServer(t *testing.T) {
	t.Parallel()

	token := fmt.Sprintf("%s:%s", handlers.AuthorizationApiKeyHeaderName, uuid.New().String())

	providerCacheDir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)

	pluginCacheDir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)

	opts := []Option{WithToken(token), WithProviderCacheDir(providerCacheDir), WithUserProviderDir(pluginCacheDir)}

	testCases := []struct {
		opts               []Option
		urlPath            string
		expectedStatusCode int
		expectedBodyReg    *regexp.Regexp
		expectedCachePath  string
	}{
		{
			opts:               opts,
			urlPath:            "/.well-known/terraform.json",
			expectedStatusCode: http.StatusOK,
			expectedBodyReg:    regexp.MustCompile(regexp.QuoteMeta(`{"providers.v1":"/v1/providers"}`)),
		},
		{
			opts:               append(opts, WithToken("")),
			urlPath:            "/v1/providers/cache/registry.terraform.io/hashicorp/aws/versions",
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			opts:               opts,
			urlPath:            "/v1/providers/cache/registry.terraform.io/hashicorp/aws/versions",
			expectedStatusCode: http.StatusOK,
			expectedBodyReg:    regexp.MustCompile(regexp.QuoteMeta(`"version":"5.36.0","protocols":["5.0"],"platforms"`)),
		},
		{
			opts:               opts,
			urlPath:            "/v1/providers/cache/registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  "registry.terraform.io/hashicorp/aws/5.36.0/darwin_arm64/terraform-provider-aws_v5.36.0_x5",
		},
		{
			opts:               opts,
			urlPath:            "/v1/providers/cache/registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  "registry.terraform.io/hashicorp/aws/5.36.0/darwin_arm64/terraform-provider-aws_v5.36.0_x5",
		},
		{
			opts:               opts,
			urlPath:            "/v1/providers/cache/registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  "registry.terraform.io/hashicorp/aws/5.36.0/darwin_arm64/terraform-provider-aws_v5.36.0_x5",
		},
		{
			opts:               opts,
			urlPath:            "/v1/providers/cache/registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  "registry.terraform.io/hashicorp/aws/5.36.0/darwin_arm64/terraform-provider-aws_v5.36.0_x5",
		},
		{
			opts:               opts,
			urlPath:            "/v1/providers/cache/registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  "registry.terraform.io/hashicorp/aws/5.36.0/darwin_arm64/terraform-provider-aws_v5.36.0_x5",
		},
		{
			opts:               opts,
			urlPath:            "/v1/providers/cache/registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  "registry.terraform.io/hashicorp/aws/5.36.0/darwin_arm64/terraform-provider-aws_v5.36.0_x5",
		},
		{
			opts:               opts,
			urlPath:            "/v1/providers/cache/registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  "registry.terraform.io/hashicorp/aws/5.36.0/darwin_arm64/terraform-provider-aws_v5.36.0_x5",
		},
		{
			opts:               opts,
			urlPath:            "/v1/providers/cache/registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  "registry.terraform.io/hashicorp/aws/5.36.0/darwin_arm64/terraform-provider-aws_v5.36.0_x5",
		},
		{
			opts:               opts,
			urlPath:            "/v1/providers/cache/registry.terraform.io/hashicorp/template/2.2.0/download/linux/amd64",
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  "registry.terraform.io/hashicorp/template/2.2.0/linux_amd64/terraform-provider-template_v2.2.0_x4",
		},
		{
			opts:               opts,
			urlPath:            fmt.Sprintf("/v1/providers/cache/registry.terraform.io/hashicorp/template/1234.5678.9/download/%s/%s", runtime.GOOS, runtime.GOARCH),
			expectedStatusCode: http.StatusLocked,
			expectedCachePath:  createFakeProvider(t, pluginCacheDir, fmt.Sprintf("registry.terraform.io/hashicorp/template/1234.5678.9/%s_%s/terraform-provider-template_1234.5678.9_x5", runtime.GOOS, runtime.GOARCH)),
		},
		{
			opts:               opts,
			urlPath:            "/v1/providers//registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusOK,
			expectedBodyReg:    regexp.MustCompile(`\{.*` + regexp.QuoteMeta(`"download_url":"http://127.0.0.1:`) + `\d+` + regexp.QuoteMeta(`/downloads/provider/releases.hashicorp.com/terraform-provider-aws/5.36.0/terraform-provider-aws_5.36.0_darwin_arm64.zip"`) + `.*\}`),
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

			server := NewServer(testCase.opts...)
			ln, err := server.Listen()
			require.NoError(t, err)
			defer ln.Close()

			errGroup.Go(func() error {
				return server.Run(ctx, ln)
			})

			urlPath := server.ProviderURL()

			urlPath.Path = testCase.urlPath

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

			server.Provider.WaitForCacheReady("")

			if testCase.expectedCachePath != "" {
				assert.FileExists(t, filepath.Join(providerCacheDir, testCase.expectedCachePath))
			}

			cancel()
			err = errGroup.Wait()
			require.NoError(t, err)
		})
	}

}
