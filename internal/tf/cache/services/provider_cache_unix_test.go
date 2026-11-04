//go:build !windows

package services_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderServiceInitWithAnotherUsersTempDir(t *testing.T) {
	t.Parallel()

	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions, so it cannot stand in for the second user")
	}

	tempDir := helpers.TmpDirWOSymlinks(t)
	require.NoError(t, os.Chmod(tempDir, os.ModeSticky|0o777))

	sharedDir := filepath.Join(tempDir, "terragrunt")
	sharedProvidersDir := filepath.Join(sharedDir, "providers")

	require.NoError(t, os.MkdirAll(sharedProvidersDir, 0o755))
	require.NoError(t, os.Chmod(sharedProvidersDir, 0o555))
	require.NoError(t, os.Chmod(sharedDir, 0o555))

	t.Cleanup(func() {
		assert.NoError(t, os.Chmod(sharedDir, 0o755))
		assert.NoError(t, os.Chmod(sharedProvidersDir, 0o755))
	})

	service := services.NewProviderService(
		helpers.TmpDirWOSymlinks(t),
		"",
		nil,
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tempDir }),
	)

	require.NoError(t, service.Init())
	assert.DirExists(t, filepath.Join(tempDir, "terragrunt-"+strconv.Itoa(os.Getuid()), "providers"))
}

func TestProviderServiceInitOnMemFS(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	service := services.NewProviderService(venvtest.Root("/cache/providers"), "", nil, logger.CreateLogger(), v)

	require.NoError(t, service.Init())

	providersDir := filepath.Join(v.Platform.TempDir(), "terragrunt-"+strconv.Itoa(v.Platform.GetUID()), "providers")
	assert.True(t, vfs.IsDir(v.FS, providersDir))
}
