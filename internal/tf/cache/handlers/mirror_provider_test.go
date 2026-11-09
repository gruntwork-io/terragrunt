package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mirrorVersionIndex = `{"archives":{"linux_amd64":{"url":"terraform-provider-example_1.0.0_linux_amd64.zip"}}}`

// TestNetworkMirrorProviderHandlerGetPlatformReportsMirrorOrigin pins that a
// package described by a network mirror is marked as one, since the cache
// service requires checksum fields from every package that is not.
func TestNetworkMirrorProviderHandlerGetPlatformReportsMirrorOrigin(t *testing.T) {
	t.Parallel()

	c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/providers/registry.example.com/example/example/1.0.0.json" {
			return vhttp.Respond(http.StatusNotFound, nil, nil), nil
		}

		return vhttp.Respond(
			http.StatusOK,
			[]byte(mirrorVersionIndex),
			http.Header{"Content-Type": []string{"application/json"}},
		), nil
	})

	handler, err := handlers.NewNetworkMirrorProviderHandler(
		logger.CreateLogger(),
		c,
		cliconfig.NewProviderInstallationNetworkMirror(
			"https://mirror.example.com/providers/",
			nil,
			nil,
		),
		nil,
	)
	require.NoError(t, err)

	body, err := handler.GetPlatform(t.Context(), mirroredProvider())
	require.NoError(t, err)
	require.NotNil(t, body)

	assert.Equal(t, models.OriginMirror, body.Origin)
}

// TestFilesystemMirrorProviderHandlerGetPlatformReportsMirrorOrigin pins that
// a package described by a filesystem mirror is marked as one, since the cache
// service requires checksum fields from every package that is not.
func TestFilesystemMirrorProviderHandlerGetPlatformReportsMirrorOrigin(t *testing.T) {
	t.Parallel()

	const mirrorPath = "/mirror"

	indexPath := filepath.Join(
		mirrorPath,
		"registry.example.com",
		"example",
		"example",
		"1.0.0.json",
	)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, fsys.MkdirAll(filepath.Dir(indexPath), 0o755))
	require.NoError(t, vfs.WriteFile(fsys, indexPath, []byte(mirrorVersionIndex), 0o644))

	handler := handlers.NewFilesystemMirrorProviderHandler(
		logger.CreateLogger(),
		vhttp.NewNoNetworkClient(),
		fsys,
		cliconfig.NewProviderInstallationFilesystemMirror(mirrorPath, nil, nil),
	)

	body, err := handler.GetPlatform(t.Context(), mirroredProvider())
	require.NoError(t, err)
	require.NotNil(t, body)

	assert.Equal(t, models.OriginMirror, body.Origin)
}

func TestFilesystemMirrorProviderHandlerGetVersions(t *testing.T) {
	t.Parallel()

	fsys := mirrorFS(t, map[string]string{
		"registry.example.com/example/example/index.json": `{"versions":{"1.0.0":{}}}`,
	})

	handler := newFilesystemMirrorHandler(fsys)
	assert.Equal(t, "filesystem_mirror '/mirror'", handler.String())

	versions, err := handler.GetVersions(t.Context(), mirroredProvider())
	require.NoError(t, err)
	require.Len(t, versions, 1)

	assert.Equal(t, "1.0.0", versions[0].Version)
	assert.NotEmpty(t, versions[0].Platforms, "a mirror version should list the platforms it may serve")
}

func TestFilesystemMirrorProviderHandlerGetPlatform(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		index           string
		wantDownloadURL string
		wantNil         bool
	}{
		{
			name:  "archive file name is joined with the mirror path",
			index: mirrorVersionIndex,
			wantDownloadURL: filepath.Join(
				"/mirror",
				"registry.example.com",
				"example",
				"example",
				"terraform-provider-example_1.0.0_linux_amd64.zip",
			),
		},
		{
			name:            "archive url is kept",
			index:           `{"archives":{"linux_amd64":{"url":"https://releases.example.com/example.zip"}}}`,
			wantDownloadURL: "https://releases.example.com/example.zip",
		},
		{
			name:    "platform the mirror does not carry",
			index:   `{"archives":{"darwin_arm64":{"url":"example.zip"}}}`,
			wantNil: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := mirrorFS(t, map[string]string{"registry.example.com/example/example/1.0.0.json": tc.index})

			body, err := newFilesystemMirrorHandler(fsys).GetPlatform(t.Context(), mirroredProvider())
			require.NoError(t, err)

			if tc.wantNil {
				assert.Nil(t, body)

				return
			}

			require.NotNil(t, body)
			assert.Equal(t, tc.wantDownloadURL, body.DownloadURL)
		})
	}
}

func TestFilesystemMirrorProviderHandlerMirrorData(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		fsys      vfs.FS
		assertErr func(t *testing.T, err error)
		name      string
	}{
		{
			name: "mirror without data for the provider",
			fsys: vfs.NewMemMapFS(),
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.NoError(t, err)
			},
		},
		{
			name: "mirror data that is not json",
			fsys: mirrorFS(t, map[string]string{
				"registry.example.com/example/example/index.json": "not json",
				"registry.example.com/example/example/1.0.0.json": "not json",
			}),
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var syntaxErr *json.SyntaxError

				require.ErrorAs(t, err, &syntaxErr)
			},
		},
		{
			name: "mirror that cannot be read",
			fsys: unreadableFS{FS: vfs.NewMemMapFS()},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, os.ErrPermission)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := newFilesystemMirrorHandler(tc.fsys)

			versions, err := handler.GetVersions(t.Context(), mirroredProvider())
			tc.assertErr(t, err)
			assert.Empty(t, versions)

			body, err := handler.GetPlatform(t.Context(), mirroredProvider())
			tc.assertErr(t, err)
			assert.Nil(t, body)
		})
	}
}

func TestNetworkMirrorProviderHandlerGetVersions(t *testing.T) {
	t.Parallel()

	c := vhttp.NewMemClient(registryRoutes(map[string]string{
		"/providers/registry.example.com/example/example/index.json": `{"versions":{"1.0.0":{}}}`,
	}))

	handler := newNetworkMirrorHandler(t, c)
	assert.Equal(t, "network_mirror 'https://mirror.example.com/providers/'", handler.String())

	versions, err := handler.GetVersions(t.Context(), mirroredProvider())
	require.NoError(t, err)
	require.Len(t, versions, 1)

	assert.Equal(t, "1.0.0", versions[0].Version)
}

func TestNetworkMirrorProviderHandlerGetPlatformResolvesArchiveURL(t *testing.T) {
	t.Parallel()

	c := vhttp.NewMemClient(registryRoutes(map[string]string{
		"/providers/registry.example.com/example/example/1.0.0.json": mirrorVersionIndex,
	}))

	body, err := newNetworkMirrorHandler(t, c).GetPlatform(t.Context(), mirroredProvider())
	require.NoError(t, err)
	require.NotNil(t, body)

	assert.Equal(
		t,
		"https://mirror.example.com/providers/registry.example.com/example/example/terraform-provider-example_1.0.0_linux_amd64.zip",
		body.DownloadURL,
		"a relative archive url should resolve under the provider address on the mirror",
	)
}

func TestNetworkMirrorProviderHandlerPlatformNotMirrored(t *testing.T) {
	t.Parallel()

	c := vhttp.NewMemClient(registryRoutes(map[string]string{
		"/providers/registry.example.com/example/example/1.0.0.json": `{"archives":{"darwin_arm64":{"url":"example.zip"}}}`,
	}))

	body, err := newNetworkMirrorHandler(t, c).GetPlatform(t.Context(), mirroredProvider())
	require.NoError(t, err)
	assert.Nil(t, body)
}

func TestNetworkMirrorProviderHandlerUnreachable(t *testing.T) {
	t.Parallel()

	handler := newNetworkMirrorHandler(t, vhttp.NewNoNetworkClient())

	versions, err := handler.GetVersions(t.Context(), mirroredProvider())
	require.ErrorIs(t, err, vhttp.ErrNoNetwork)
	assert.Nil(t, versions)

	body, err := handler.GetPlatform(t.Context(), mirroredProvider())
	require.ErrorIs(t, err, vhttp.ErrNoNetwork)
	assert.Nil(t, body)
}

func TestNewNetworkMirrorProviderHandlerInvalidURL(t *testing.T) {
	t.Parallel()

	handler, err := handlers.NewNetworkMirrorProviderHandler(
		logger.CreateLogger(),
		vhttp.NewNoNetworkClient(),
		cliconfig.NewProviderInstallationNetworkMirror("https://bad host/providers/", nil, nil),
		nil,
	)

	var urlErr *url.Error

	require.ErrorAs(t, err, &urlErr)
	assert.Nil(t, handler)
}

func mirroredProvider() *models.Provider {
	return &models.Provider{
		RegistryName: "registry.example.com",
		Namespace:    "example",
		Name:         "example",
		Version:      "1.0.0",
		OS:           "linux",
		Arch:         "amd64",
	}
}

// unreadableFS refuses to open any file.
type unreadableFS struct {
	vfs.FS
}

func (unreadableFS) Open(string) (vfs.File, error) {
	return nil, os.ErrPermission
}

// mirrorFS returns an in-memory filesystem mirror at /mirror holding files.
func mirrorFS(t *testing.T, files map[string]string) vfs.FS {
	t.Helper()

	fsys := vfs.NewMemMapFS()

	for name, content := range files {
		path := filepath.Join("/mirror", filepath.FromSlash(name))

		require.NoError(t, fsys.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
	}

	return fsys
}

// newFilesystemMirrorHandler returns a handler for the filesystem mirror at /mirror.
func newFilesystemMirrorHandler(fsys vfs.FS) *handlers.FilesystemMirrorProviderHandler {
	return handlers.NewFilesystemMirrorProviderHandler(
		logger.CreateLogger(),
		vhttp.NewNoNetworkClient(),
		fsys,
		cliconfig.NewProviderInstallationFilesystemMirror("/mirror", nil, nil),
	)
}

// newNetworkMirrorHandler returns a handler for the network mirror at mirror.example.com.
func newNetworkMirrorHandler(t *testing.T, c vhttp.Client) *handlers.NetworkMirrorProviderHandler {
	t.Helper()

	handler, err := handlers.NewNetworkMirrorProviderHandler(
		logger.CreateLogger(),
		c,
		cliconfig.NewProviderInstallationNetworkMirror("https://mirror.example.com/providers/", nil, nil),
		nil,
	)
	require.NoError(t, err)

	return handler
}
