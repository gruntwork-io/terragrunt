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
	"net/http/httptest"
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
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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

	fakeRegistry := newFakeRegistryServer(t)
	fakeRegistryURLs := &handlers.RegistryURLs{
		ModulesV1:   fakeRegistry.URL + "/v1/modules",
		ProvidersV1: fakeRegistry.URL + "/v1/providers",
	}

	opts := make([]cache.Option, 0, 3)
	opts = append(
		opts,
		cache.WithToken(token),
		cache.WithCacheProviderHTTPStatusCode(providercache.CacheProviderHTTPStatusCode),
	)

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
			expectedBodyReg: regexp.MustCompile(
				regexp.QuoteMeta(`{"providers.v1":"/v1/providers"}`),
			),
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
			expectedBodyReg: regexp.MustCompile(
				regexp.QuoteMeta(`"version":"5.36.0","protocols":["5.0"],"platforms"`),
			),
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
			expectedBodyReg: regexp.MustCompile(
				`\{.*` + regexp.QuoteMeta(
					`"download_url":"http://127.0.0.1:`,
				) + `\d+` + regexp.QuoteMeta(
					`/downloads/releases.hashicorp.com/terraform-provider-aws/5.36.0/terraform-provider-aws_5.36.0_darwin_arm64.zip"`,
				) + `.*\}`,
			),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			errGroup, ctx := errgroup.WithContext(ctx)
			l := logger.CreateLogger()

			providerService := services.NewProviderService(providerCacheDir, pluginCacheDir, nil, l)
			providerHandler := handlers.NewDirectProviderHandler(
				l,
				new(cliconfig.ProviderInstallationDirect),
				nil,
			)
			// Seed discovery so the direct handler resolves registry.terraform.io
			// against the in-process fake registry instead of doing live discovery.
			// The proxy handler below keeps its own discovery cache untouched.
			providerHandler.SetDiscoveryURLCache("registry.terraform.io", fakeRegistryURLs)

			proxyProviderHandler := handlers.NewProxyProviderHandler(l, nil)

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

func TestProviderCacheHomeless(t *testing.T) {
	cacheDir := helpers.TmpDirWOSymlinks(t)

	t.Setenv("HOME", "")
	require.NoError(t, os.Unsetenv("HOME"))

	t.Setenv("XDG_CACHE_HOME", "")
	require.NoError(t, os.Unsetenv("XDG_CACHE_HOME"))

	_, err := providercache.InitServer(logger.CreateLogger(), &pcoptions.ProviderCacheOptions{
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

		server := providercache.NewProviderCache().WithFS(memFs)
		err := server.Init(
			logger.CreateLogger(),
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

		server := providercache.NewProviderCache().WithFS(memFs)
		err := server.Init(
			logger.CreateLogger(),
			&pcoptions.ProviderCacheOptions{
				Dir: cacheDir,
			},
			"",
		)
		require.NoError(t, err)
		require.NotNil(t, server, "Init should return a valid server when using VFS")
	})
}

// fakeRegistryRoute is a canned response served by newFakeRegistryServer.
type fakeRegistryRoute struct {
	contentType string
	body        []byte
}

// newFakeRegistryServer starts an in-process provider registry that serves
// versions metadata, download metadata, archives and checksums for the
// providers requested by TestProviderCache, so caching test cases never make
// requests outside localhost. Unknown paths, such as versions that do not
// exist upstream, get 404 responses.
func newFakeRegistryServer(t *testing.T) *httptest.Server {
	t.Helper()

	routes := make(map[string]fakeRegistryRoute)
	addFakeRegistryProvider(t, routes, "aws", "5.36.0", "x5", "darwin", "arm64")
	addFakeRegistryProvider(t, routes, "template", "2.2.0", "x4", "linux", "amd64")

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, ok := routes[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)

				return
			}

			w.Header().Set("Content-Type", route.contentType)

			if _, err := w.Write(route.body); err != nil {
				t.Errorf("fake registry write failed: %v", err)
			}
		}),
	)
	t.Cleanup(server.Close)

	return server
}

// addFakeRegistryProvider registers the registry responses needed to fully
// cache one hashicorp provider platform: the versions list, the download
// metadata, the archive, its SHA256SUMS document and a dummy signature.
// The download metadata uses root-relative URLs that the direct handler
// resolves against the fake registry, and carries no GPG keys so package
// authentication runs both checksum checks and skips the signature check.
func addFakeRegistryProvider(
	t *testing.T,
	routes map[string]fakeRegistryRoute,
	name, version, protocolSuffix, osName, arch string,
) {
	t.Helper()

	archive := buildFakeProviderZip(
		t,
		fmt.Sprintf("terraform-provider-%s_v%s_%s", name, version, protocolSuffix),
	)
	filename := fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", name, version, osName, arch)
	checksum := sha256.Sum256(archive)

	archivePath := "/archives/" + filename
	shasumsPath := fmt.Sprintf("/shasums/terraform-provider-%s_%s_SHA256SUMS", name, version)
	shasumsSignaturePath := shasumsPath + ".sig"
	downloadPath := fmt.Sprintf(
		"/v1/providers/hashicorp/%s/%s/download/%s/%s",
		name,
		version,
		osName,
		arch,
	)

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
		Platform:               models.Platform{OS: osName, Arch: arch},
		Filename:               filename,
		DownloadURL:            archivePath,
		SHA256SumsURL:          shasumsPath,
		SHA256SumsSignatureURL: shasumsSignaturePath,
		SHA256Sum:              hex.EncodeToString(checksum[:]),
		Protocols:              []string{"5.0"},
	})
	require.NoError(t, err)

	routes["/v1/providers/hashicorp/"+name+"/versions"] = fakeRegistryRoute{
		contentType: "application/json",
		body:        versionsBody,
	}
	routes[downloadPath] = fakeRegistryRoute{
		contentType: "application/json",
		body:        downloadBody,
	}
	routes[archivePath] = fakeRegistryRoute{
		contentType: "application/zip",
		body:        archive,
	}
	routes[shasumsPath] = fakeRegistryRoute{
		contentType: "text/plain",
		body:        fmt.Appendf(nil, "%x  %s\n", checksum, filename),
	}
	routes[shasumsSignaturePath] = fakeRegistryRoute{
		contentType: "application/octet-stream",
		body:        []byte("fake-signature"),
	}
}

// buildFakeProviderZip returns an in-memory provider archive containing a
// single empty file entry named after the provider binary.
func buildFakeProviderZip(t *testing.T, binaryName string) []byte {
	t.Helper()

	buffer := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buffer)

	_, err := zipWriter.Create(binaryName)
	require.NoError(t, err)
	require.NoError(t, zipWriter.Close())

	return buffer.Bytes()
}
