package services_test

import (
	"errors"
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestRemoveStaleSymlink(t *testing.T) {
	t.Parallel()

	const path = "/cache/registry.terraform.io/hashicorp/aws/5.31.0/linux_amd64"

	testCases := []struct {
		setup     func(t *testing.T, fsys vfs.FS)
		assertErr func(t *testing.T, err error)
		assertFS  func(t *testing.T, fsys vfs.FS)
		name      string
	}{
		{
			name:  "no entry returns nil",
			setup: func(t *testing.T, fsys vfs.FS) { t.Helper() },
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
			assertFS: func(t *testing.T, fsys vfs.FS) {
				t.Helper()

				_, err := vfs.Lstat(fsys, path)
				assert.ErrorIs(t, err, os.ErrNotExist, "expected NotExist, got %v", err)
			},
		},
		{
			name: "dangling symlink is removed",
			setup: func(t *testing.T, fsys vfs.FS) {
				t.Helper()
				require.NoError(t, vfs.Symlink(fsys, "/missing/target", path))
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
			assertFS: func(t *testing.T, fsys vfs.FS) {
				t.Helper()

				_, err := vfs.Lstat(fsys, path)
				assert.ErrorIs(
					t,
					err,
					os.ErrNotExist,
					"expected NotExist after remove, got %v",
					err,
				)
			},
		},
		{
			name: "regular file returns typed error and is left in place",
			setup: func(t *testing.T, fsys vfs.FS) {
				t.Helper()
				require.NoError(
					t,
					fsys.MkdirAll("/cache/registry.terraform.io/hashicorp/aws/5.31.0", 0o755),
				)
				require.NoError(t, afero.WriteFile(fsys, path, []byte("user content"), 0o644))
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var unexpected *services.UnexpectedProviderCachePathError

				require.ErrorAs(t, err, &unexpected)
				assert.Equal(t, path, unexpected.Path)
				assert.Zero(t, unexpected.Mode&os.ModeSymlink)
			},
			assertFS: func(t *testing.T, fsys vfs.FS) {
				t.Helper()

				exists, err := vfs.FileExists(fsys, path)
				require.NoError(t, err)
				assert.True(t, exists, "regular file must not be deleted")
			},
		},
		{
			name: "regular directory returns typed error and is left in place",
			setup: func(t *testing.T, fsys vfs.FS) {
				t.Helper()
				require.NoError(t, fsys.MkdirAll(path, 0o755))
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var unexpected *services.UnexpectedProviderCachePathError

				require.ErrorAs(t, err, &unexpected)
				assert.Equal(t, path, unexpected.Path)
				assert.True(t, unexpected.Mode.IsDir())
				assert.Zero(t, unexpected.Mode&os.ModeSymlink)
			},
			assertFS: func(t *testing.T, fsys vfs.FS) {
				t.Helper()

				exists, err := vfs.FileExists(fsys, path)
				require.NoError(t, err)
				assert.True(t, exists, "directory must not be deleted")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			tc.setup(t, fsys)

			tc.assertErr(t, services.RemoveStaleSymlink(fsys, path))
			tc.assertFS(t, fsys)
		})
	}
}

func TestRemoveStaleSymlinkLstatErrorIsWrapped(t *testing.T) {
	t.Parallel()

	wantInner := errors.New("synthetic lstat failure")
	fsys := &lstatErrorFS{FS: vfs.NewMemMapFS(), err: wantInner}

	err := services.RemoveStaleSymlink(fsys, "/anything")
	require.Error(t, err)
	assert.ErrorIs(t, err, wantInner)
}

type lstatErrorFS struct {
	vfs.FS
	err error
}

func (fsys *lstatErrorFS) LstatIfPossible(string) (os.FileInfo, bool, error) {
	return nil, false, fsys.err
}

// TestProviderServiceInitIsIdempotent covers the server initializing a
// service that a caller may also drive through Run: the directories are
// created once, and both callers see the same outcome.
func TestProviderServiceInitIsIdempotent(t *testing.T) {
	t.Parallel()

	service := services.NewProviderService(
		t.TempDir(), t.TempDir(), nil, logger.CreateLogger(), venvtest.NewOSWithEmptyEnv(),
	)

	require.NoError(t, service.Init())
	require.NoError(t, service.Init())
}

func TestProviderServiceInitWithoutCacheDir(t *testing.T) {
	t.Parallel()

	service := services.NewProviderService(
		"", t.TempDir(), nil, logger.CreateLogger(), venvtest.NewOSWithEmptyEnv(),
	)

	require.ErrorIs(t, service.Init(), services.ErrCacheDirNotSpecified)
	require.ErrorIs(t, service.Run(t.Context()), services.ErrCacheDirNotSpecified)
}
