//go:build !windows

package services_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProviderServiceInitWithAnotherUsersTempDir covers two users sharing a
// temp directory: the terragrunt/providers directory the first user left there
// does not stop the second user's service from starting.
func TestProviderServiceInitWithAnotherUsersTempDir(t *testing.T) {
	t.Parallel()

	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions, so it cannot stand in for the second user")
	}

	tempDir := helpers.TmpDirWOSymlinks(t)
	sharedDir := filepath.Join(tempDir, "terragrunt")
	sharedProvidersDir := filepath.Join(sharedDir, "providers")

	// Read-only directories stand in for ones another user owns.
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
