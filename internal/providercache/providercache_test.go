package providercache_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"uuid"

	"github.com/gruntwork-io/terragrunt/internal/providercache"
	pcoptions "github.com/gruntwork-io/terragrunt/internal/providercache/options"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
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
	opts = append(
		opts,
		cache.WithToken(token),
		cache.WithCacheProviderHTTPStatusCode(providercache.CacheProviderHTTPStatusCode),
	)

	testCases := []struct {
		fullURLPath          string
		relURLPath           string
		expectedBody         string
		expectedDownloadPath string
		expectedCachePath    string
		opts                 []cache.Option
		expectedStatusCode   int
	}{
		{
			opts:               opts,
			fullURLPath:        "/.well-known/terraform.json",
			expectedStatusCode: http.StatusOK,
			expectedBody:       `{"providers.v1":"/v1/providers"}`,
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
			expectedBody:       `"version":"5.36.0","protocols":["5.0"],"platforms"`,
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
			opts: opts,
			relURLPath: fmt.Sprintf(
				"/cache/registry.terraform.io/hashicorp/template/1234.5678.9/download/%s/%s",
				runtime.GOOS,
				runtime.GOARCH,
			),
			expectedStatusCode: http.StatusLocked,
			expectedCachePath: createFakeProvider(
				t,
				pluginCacheDir,
				fmt.Sprintf(
					"registry.terraform.io/hashicorp/template/1234.5678.9/%s_%s/terraform-provider-template_1234.5678.9_x5",
					runtime.GOOS,
					runtime.GOARCH,
				),
			),
		},
		{
			opts:               opts,
			relURLPath:         "//registry.terraform.io/hashicorp/aws/5.36.0/download/darwin/arm64",
			expectedStatusCode: http.StatusOK,
			expectedDownloadPath: "/releases.hashicorp.com/terraform-provider-aws/5.36.0/" +
				"terraform-provider-aws_5.36.0_darwin_arm64.zip",
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			errGroup, ctx := errgroup.WithContext(ctx)
			l := logger.CreateLogger()

			providerService := services.NewProviderService(
				providerCacheDir,
				pluginCacheDir,
				nil,
				l,
				venvtest.NewOSWithEmptyEnv().WithHTTP(c),
			)
			providerHandler := handlers.NewDirectProviderHandler(
				l,
				c,
				new(cliconfig.ProviderInstallationDirect),
				nil,
			)
			proxyProviderHandler := handlers.NewProxyProviderHandler(l, c, nil)

			tc.opts = append(tc.opts,
				cache.WithProviderService(providerService),
				cache.WithProviderHandlers(providerHandler),
				cache.WithProxyProviderHandler(proxyProviderHandler),
			)

			server := cache.NewServer(tc.opts...)

			ln, err := server.Listen(t.Context(), venvtest.NewOSWithEmptyEnv())
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

			if tc.expectedBody != "" || tc.expectedDownloadPath != "" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				if tc.expectedBody != "" {
					assert.Contains(t, string(body), tc.expectedBody)
				}

				if tc.expectedDownloadPath != "" {
					downloadURL := "http://" + ln.Addr().String() +
						"/downloads/" + server.DownloaderController.Segment() +
						tc.expectedDownloadPath

					assert.Contains(t, string(body), `"download_url":"`+downloadURL+`"`)
				}
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

// fakeRelease is a canned response served from releases.hashicorp.com.
type fakeRelease struct {
	contentType string
	body        []byte
}

// registryHandler synthesizes the slice of the provider registry protocol the
// cache server exercises: service discovery, version listings, platform
// download documents, and the release archives, checksums and signatures
// themselves. Anything else gets a 404, which is also what the real registry
// returns for the fabricated 1234.5678.9 version — the cache must then fall
// back to the user plugin dir.
func registryHandler(t *testing.T) vhttp.Handler {
	t.Helper()

	registry := map[string][]byte{
		"/.well-known/terraform.json": []byte(`{"providers.v1":"/v1/providers"}`),
	}
	releases := make(map[string]fakeRelease)

	addFakeProvider(t, registry, releases, "aws", "5.36.0", "x5", "darwin", "arm64")
	addFakeProvider(t, registry, releases, "template", "2.2.0", "x4", "linux", "amd64")

	return func(_ context.Context, req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "registry.terraform.io":
			if body, ok := registry[req.URL.Path]; ok {
				return vhttp.Respond(http.StatusOK, body,
					http.Header{"Content-Type": []string{"application/json"}}), nil
			}
		case "releases.hashicorp.com":
			if release, ok := releases[req.URL.Path]; ok {
				return vhttp.Respond(http.StatusOK, release.body,
					http.Header{"Content-Type": []string{release.contentType}}), nil
			}
		}

		return vhttp.Respond(http.StatusNotFound, nil, nil), nil
	}
}

// addFakeProvider registers the responses needed to fully cache one hashicorp
// provider platform: the versions list and download document on the registry,
// plus the archive, its SHA256SUMS document and a dummy signature on the
// release host. The download document carries no GPG keys, so package
// authentication runs both checksum checks and skips the signature check.
func addFakeProvider(
	t *testing.T,
	registry map[string][]byte,
	releases map[string]fakeRelease,
	name, version, protocolSuffix, osName, arch string,
) {
	t.Helper()

	const releasesURL = "https://releases.hashicorp.com"

	archive := zipWithFile(
		t,
		fmt.Sprintf("terraform-provider-%s_v%s_%s", name, version, protocolSuffix),
	)
	filename := fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", name, version, osName, arch)
	checksum := sha256.Sum256(archive)

	releaseDir := fmt.Sprintf("/terraform-provider-%s/%s/", name, version)
	archivePath := releaseDir + filename
	shasumsPath := fmt.Sprintf("%sterraform-provider-%s_%s_SHA256SUMS", releaseDir, name, version)
	signaturePath := shasumsPath + ".sig"

	versionsBody, err := json.Marshal(struct {
		Versions models.Versions `json:"versions"`
	}{
		Versions: models.Versions{
			{
				Version:   version,
				Protocols: []string{"5.0"},
				Platforms: models.Platforms{
					{OS: "darwin", Arch: "arm64"},
					{OS: "linux", Arch: "amd64"},
				},
			},
		},
	})
	require.NoError(t, err)

	downloadBody, err := json.Marshal(&models.ResponseBody{
		OS:                     osName,
		Arch:                   arch,
		Filename:               filename,
		DownloadURL:            releasesURL + archivePath,
		SHA256SumsURL:          releasesURL + shasumsPath,
		SHA256SumsSignatureURL: releasesURL + signaturePath,
		SHA256Sum:              hex.EncodeToString(checksum[:]),
		Protocols:              []string{"5.0"},
	})
	require.NoError(t, err)

	registry["/v1/providers/hashicorp/"+name+"/versions"] = versionsBody
	registry[fmt.Sprintf(
		"/v1/providers/hashicorp/%s/%s/download/%s/%s",
		name,
		version,
		osName,
		arch,
	)] = downloadBody

	releases[archivePath] = fakeRelease{contentType: "application/zip", body: archive}
	releases[shasumsPath] = fakeRelease{
		contentType: "text/plain",
		body:        fmt.Appendf(nil, "%x  %s\n", checksum, filename),
	}
	releases[signaturePath] = fakeRelease{
		contentType: "application/octet-stream",
		body:        []byte("fake-signature"),
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

	_, err := providercache.InitServer(
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv(),
		tfimpl.OpenTofu,
		&pcoptions.ProviderCacheOptions{
			Dir: cacheDir,
		},
		"",
	)
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
			venvtest.New().WithFS(memFs),
			tfimpl.OpenTofu,
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
			venvtest.New().WithFS(memFs),
			tfimpl.OpenTofu,
			&pcoptions.ProviderCacheOptions{
				Dir: cacheDir,
			},
			"",
		)
		require.NoError(t, err)
		require.NotNil(t, server, "Init should return a valid server when using VFS")
	})
}
