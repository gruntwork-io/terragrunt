//go:build !windows

package services_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
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

// TestProviderServiceInitFallsBackFromUnsafeTempDir checks that an unsafe shared temp dir costs only
// lock sharing: Init uses a private per-run dir, never touches the unsafe path, and Run removes it.
func TestProviderServiceInitFallsBackFromUnsafeTempDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		setup func(t *testing.T, tempDir, sharedDir string)
		name  string
	}{
		{
			// A Kubernetes emptyDir mounted at /tmp is world-writable without the sticky bit.
			name: "world-writable temp dir without sticky bit",
			setup: func(t *testing.T, tempDir, _ string) {
				t.Helper()

				require.NoError(t, os.Chmod(tempDir, 0o777))
			},
		},
		{
			name: "shared dir squatted by a symlink",
			setup: func(t *testing.T, tempDir, sharedDir string) {
				t.Helper()

				require.NoError(t, os.Chmod(tempDir, os.ModeSticky|0o777))
				require.NoError(t, os.Mkdir(filepath.Join(tempDir, "target"), 0o700))
				require.NoError(t, os.Symlink(filepath.Join(tempDir, "target"), sharedDir))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := helpers.TmpDirWOSymlinks(t)
			sharedDir := filepath.Join(tempDir, "terragrunt-"+strconv.Itoa(os.Getuid()))

			tc.setup(t, tempDir, sharedDir)

			service := services.NewProviderService(
				helpers.TmpDirWOSymlinks(t),
				"",
				nil,
				logger.CreateLogger(),
				venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tempDir }),
			)

			require.NoError(t, service.Init())

			runDirs, err := filepath.Glob(filepath.Join(tempDir, "terragrunt-*"))
			require.NoError(t, err)

			runDirs = slices.DeleteFunc(runDirs, func(dir string) bool { return dir == sharedDir })
			require.Len(t, runDirs, 1)

			runDir := runDirs[0]

			info, err := os.Lstat(runDir)
			require.NoError(t, err)
			assert.True(t, info.IsDir())
			assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
			assert.DirExists(t, filepath.Join(runDir, "providers"))
			assert.NoDirExists(t, filepath.Join(tempDir, "target", "providers"))

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			require.NoError(t, service.Run(ctx))
			assert.NoDirExists(t, runDir)
		})
	}
}

func TestProviderServiceInitOnMemFS(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	service := services.NewProviderService(venvtest.Root("/cache/providers"), "", nil, logger.CreateLogger(), v)

	require.NoError(t, service.Init())

	providersDir := filepath.Join(v.Platform.TempDir(), "terragrunt-"+strconv.Itoa(v.Platform.GetUID()), "providers")
	assert.True(t, vfs.IsDir(v.FS, providersDir))
}
