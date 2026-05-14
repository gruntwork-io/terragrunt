package services_test

import (
	"errors"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveStaleSymlink(t *testing.T) {
	t.Parallel()

	const path = "/cache/registry.terraform.io/hashicorp/aws/5.31.0/linux_amd64"

	testCases := []struct {
		setup     func(t *testing.T, fs vfs.FS)
		assertErr func(t *testing.T, err error)
		assertFS  func(t *testing.T, fs vfs.FS)
		name      string
	}{
		{
			name:  "no entry returns nil",
			setup: func(t *testing.T, fs vfs.FS) { t.Helper() },
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
			assertFS: func(t *testing.T, fs vfs.FS) {
				t.Helper()

				_, err := vfs.Lstat(fs, path)
				assert.True(t, os.IsNotExist(err), "expected NotExist, got %v", err)
			},
		},
		{
			name: "dangling symlink is removed",
			setup: func(t *testing.T, fs vfs.FS) {
				t.Helper()
				require.NoError(t, vfs.Symlink(fs, "/missing/target", path))
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
			assertFS: func(t *testing.T, fs vfs.FS) {
				t.Helper()

				_, err := vfs.Lstat(fs, path)
				assert.True(t, os.IsNotExist(err), "expected NotExist after remove, got %v", err)
			},
		},
		{
			name: "regular file returns typed error and is left in place",
			setup: func(t *testing.T, fs vfs.FS) {
				t.Helper()
				require.NoError(t, fs.MkdirAll("/cache/registry.terraform.io/hashicorp/aws/5.31.0", 0o755))
				require.NoError(t, afero.WriteFile(fs, path, []byte("user content"), 0o644))
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var unexpected *services.UnexpectedProviderCachePathError

				require.ErrorAs(t, err, &unexpected)
				assert.Equal(t, path, unexpected.Path)
				assert.Zero(t, unexpected.Mode&os.ModeSymlink)
			},
			assertFS: func(t *testing.T, fs vfs.FS) {
				t.Helper()

				exists, err := vfs.FileExists(fs, path)
				require.NoError(t, err)
				assert.True(t, exists, "regular file must not be deleted")
			},
		},
		{
			name: "regular directory returns typed error and is left in place",
			setup: func(t *testing.T, fs vfs.FS) {
				t.Helper()
				require.NoError(t, fs.MkdirAll(path, 0o755))
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var unexpected *services.UnexpectedProviderCachePathError

				require.ErrorAs(t, err, &unexpected)
				assert.Equal(t, path, unexpected.Path)
				assert.True(t, unexpected.Mode.IsDir())
				assert.Zero(t, unexpected.Mode&os.ModeSymlink)
			},
			assertFS: func(t *testing.T, fs vfs.FS) {
				t.Helper()

				exists, err := vfs.FileExists(fs, path)
				require.NoError(t, err)
				assert.True(t, exists, "directory must not be deleted")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()
			tc.setup(t, fs)

			tc.assertErr(t, services.RemoveStaleSymlink(fs, path))
			tc.assertFS(t, fs)
		})
	}
}

func TestRemoveStaleSymlinkLstatErrorIsWrapped(t *testing.T) {
	t.Parallel()

	wantInner := errors.New("synthetic lstat failure")
	fs := &lstatErrorFS{FS: vfs.NewMemMapFS(), err: wantInner}

	err := services.RemoveStaleSymlink(fs, "/anything")
	require.Error(t, err)
	assert.ErrorIs(t, err, wantInner)
}

type lstatErrorFS struct {
	vfs.FS
	err error
}

func (fs *lstatErrorFS) LstatIfPossible(string) (os.FileInfo, bool, error) {
	return nil, false, fs.err
}
