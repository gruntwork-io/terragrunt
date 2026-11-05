//go:build !windows

package services_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
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

	service := newProviderService(t, venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tempDir }))

	require.NoError(t, service.Init())
	assert.DirExists(t, filepath.Join(tempDir, "terragrunt-"+strconv.Itoa(os.Getuid()), "providers"))
}

func TestProviderServiceInitFallsBackFromUnsafeTempDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		setup func(t *testing.T, tempDir, sharedDir string)
		name  string
	}{
		{
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
			userCacheDir := helpers.TmpDirWOSymlinks(t)

			tc.setup(t, tempDir, filepath.Join(tempDir, "terragrunt-"+strconv.Itoa(os.Getuid())))

			tempDirEntries := dirNames(t, tempDir)

			v := venvtest.NewOSWithEmptyEnv().
				WithTempDir(func() string { return tempDir }).
				WithUserCacheDir(func() (string, error) { return userCacheDir, nil })

			first := newProviderService(t, v)
			second := newProviderService(t, v)

			require.NoError(t, first.Init())
			require.NoError(t, second.Init())

			assert.Equal(t, tempDirEntries, dirNames(t, tempDir))
			assert.NoDirExists(t, filepath.Join(tempDir, "target", "providers"))

			fallbackInfo, err := os.Stat(filepath.Join(userCacheDir, "terragrunt", "tmp"))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o700), fallbackInfo.Mode().Perm())

			archiveDirsGlob := filepath.Join(userCacheDir, "terragrunt", "tmp", "providers", "archives-*")

			archiveDirs, err := filepath.Glob(archiveDirsGlob)
			require.NoError(t, err)
			assert.Len(t, archiveDirs, 2)

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			require.NoError(t, first.Run(ctx))
			require.NoError(t, second.Run(ctx))

			archiveDirs, err = filepath.Glob(archiveDirsGlob)
			require.NoError(t, err)
			assert.Empty(t, archiveDirs)
		})
	}
}

func TestProviderServiceInitFailsWithoutSafeTempDir(t *testing.T) {
	t.Parallel()

	tempDir := helpers.TmpDirWOSymlinks(t)
	require.NoError(t, os.Chmod(tempDir, 0o777))

	errNoUserCacheDir := errors.New("no user cache dir")

	service := newProviderService(
		t,
		venvtest.NewOSWithEmptyEnv().
			WithTempDir(func() string { return tempDir }).
			WithUserCacheDir(func() (string, error) { return "", errNoUserCacheDir }),
	)

	err := service.Init()

	var unsafeErr util.UnsafeTempDirError

	require.ErrorAs(t, err, &unsafeErr)
	require.ErrorIs(t, err, errNoUserCacheDir)
	require.ErrorIs(t, err, services.ErrNoSafeTempDir)
	assert.Empty(t, dirNames(t, tempDir))
}

func TestProviderServiceInitOnMemFS(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	service := services.NewProviderService(venvtest.Root("/cache/providers"), "", nil, logger.CreateLogger(), v)

	require.NoError(t, service.Init())

	providersDir := filepath.Join(v.Platform.TempDir(), "terragrunt-"+strconv.Itoa(v.Platform.GetUID()), "providers")
	assert.True(t, vfs.IsDir(v.FS, providersDir))
}

func newProviderService(t *testing.T, v *venv.Venv) *services.ProviderService {
	t.Helper()

	return services.NewProviderService(helpers.TmpDirWOSymlinks(t), "", nil, logger.CreateLogger(), v)
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}
