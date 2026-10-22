package handlers_test

import (
	"context"
	"net/http"
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
