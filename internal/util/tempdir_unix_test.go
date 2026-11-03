//go:build !windows

package util_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnsureTempDir covers a temp directory shared by several users: each user
// gets a directory of their own, and one that another user could have planted
// or can still write to is refused.
func TestEnsureTempDir(t *testing.T) {
	t.Parallel()

	const marker = "marker"

	testCases := []struct {
		setup   func(t *testing.T, dir string)
		check   func(t *testing.T, dir string)
		name    string
		uidStep int
		wantErr bool
	}{
		{
			name: "missing is created private",
			check: func(t *testing.T, dir string) {
				t.Helper()

				info, err := os.Lstat(dir)
				require.NoError(t, err)
				assert.True(t, info.IsDir())
				assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
			},
		},
		{
			name: "existing private is reused",
			setup: func(t *testing.T, dir string) {
				t.Helper()

				require.NoError(t, os.Mkdir(dir, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, marker), nil, 0o600))
			},
			check: func(t *testing.T, dir string) {
				t.Helper()

				assert.FileExists(t, filepath.Join(dir, marker))
			},
		},
		{
			name: "group writable",
			setup: func(t *testing.T, dir string) {
				t.Helper()

				require.NoError(t, os.Mkdir(dir, 0o700))
				require.NoError(t, os.Chmod(dir, 0o770))
			},
			wantErr: true,
		},
		{
			name: "world writable with sticky bit",
			setup: func(t *testing.T, dir string) {
				t.Helper()

				require.NoError(t, os.Mkdir(dir, 0o700))
				require.NoError(t, os.Chmod(dir, os.ModeSticky|0o777))
			},
			wantErr: true,
		},
		{
			name: "symlink to a private directory",
			setup: func(t *testing.T, dir string) {
				t.Helper()

				target := filepath.Join(filepath.Dir(dir), "target")
				require.NoError(t, os.Mkdir(target, 0o700))
				require.NoError(t, os.Symlink(target, dir))
			},
			wantErr: true,
		},
		{
			name: "dangling symlink",
			setup: func(t *testing.T, dir string) {
				t.Helper()

				require.NoError(t, os.Symlink(filepath.Join(filepath.Dir(dir), "missing"), dir))
			},
			wantErr: true,
		},
		{
			name: "regular file",
			setup: func(t *testing.T, dir string) {
				t.Helper()

				require.NoError(t, os.WriteFile(dir, nil, 0o600))
			},
			wantErr: true,
		},
		{
			name:    "owned by another user",
			uidStep: 1,
			setup: func(t *testing.T, dir string) {
				t.Helper()

				require.NoError(t, os.Mkdir(dir, 0o700))
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := helpers.TmpDirWOSymlinks(t)

			// A uid other than the real one stands in for the user the
			// directory belongs to, since a test cannot create files as
			// someone else.
			uid := os.Getuid() + tc.uidStep
			want := filepath.Join(tempDir, "terragrunt-"+strconv.Itoa(uid))

			if tc.setup != nil {
				tc.setup(t, want)
			}

			v := venvtest.NewOSWithEmptyEnv().
				WithTempDir(func() string { return tempDir }).
				WithGetUID(func() int { return uid })

			got, err := util.EnsureTempDir(v)

			if tc.wantErr {
				var unsafeErr util.UnsafeTempDirError

				require.ErrorAs(t, err, &unsafeErr)
				assert.Equal(t, want, unsafeErr.Path)
				assert.Empty(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, want, got)

			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// TestEnsureTempDirCreatesMissingTempDir keeps a temp directory that does not
// exist yet working, as it did before the directory was made per user.
func TestEnsureTempDirCreatesMissingTempDir(t *testing.T) {
	t.Parallel()

	tempDir := filepath.Join(helpers.TmpDirWOSymlinks(t), "missing", "tmp")

	got, err := util.EnsureTempDir(
		venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tempDir }),
	)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDir, "terragrunt-"+strconv.Itoa(os.Getuid())), got)
	assert.DirExists(t, got)
}
