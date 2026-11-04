//go:build !windows

package util_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureTempDir(t *testing.T) {
	t.Parallel()

	const marker = "marker"

	testCases := []struct {
		setup   func(t *testing.T, parentDir, dir string)
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
			setup: func(t *testing.T, _, dir string) {
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
			name: "writable by group",
			setup: func(t *testing.T, _, dir string) {
				t.Helper()

				require.NoError(t, os.Mkdir(dir, 0o700))
				require.NoError(t, os.Chmod(dir, 0o770))
			},
			wantErr: true,
		},
		{
			name: "world writable with sticky bit",
			setup: func(t *testing.T, _, dir string) {
				t.Helper()

				require.NoError(t, os.Mkdir(dir, 0o700))
				require.NoError(t, os.Chmod(dir, os.ModeSticky|0o777))
			},
			wantErr: true,
		},
		{
			name: "symlink to a private directory",
			setup: func(t *testing.T, _, dir string) {
				t.Helper()

				target := filepath.Join(filepath.Dir(dir), "target")
				require.NoError(t, os.Mkdir(target, 0o700))
				require.NoError(t, os.Symlink(target, dir))
			},
			wantErr: true,
		},
		{
			name: "dangling symlink",
			setup: func(t *testing.T, _, dir string) {
				t.Helper()

				require.NoError(t, os.Symlink(filepath.Join(filepath.Dir(dir), "missing"), dir))
			},
			wantErr: true,
		},
		{
			name: "regular file",
			setup: func(t *testing.T, _, dir string) {
				t.Helper()

				require.NoError(t, os.WriteFile(dir, nil, 0o600))
			},
			wantErr: true,
		},
		{
			name:    "owned by another user",
			uidStep: 1,
			setup: func(t *testing.T, _, dir string) {
				t.Helper()

				require.NoError(t, os.Mkdir(dir, 0o700))
			},
			wantErr: true,
		},
		{
			name: "parent writable by others without sticky bit",
			setup: func(t *testing.T, parentDir, _ string) {
				t.Helper()

				require.NoError(t, os.Chmod(parentDir, 0o777))
			},
			check:   assertNoDir,
			wantErr: true,
		},
		{
			name: "parent writable by group without sticky bit",
			setup: func(t *testing.T, parentDir, _ string) {
				t.Helper()

				require.NoError(t, os.Chmod(parentDir, 0o770))
			},
			check:   assertNoDir,
			wantErr: true,
		},
		{
			name: "grandparent writable by others without sticky bit",
			setup: func(t *testing.T, parentDir, _ string) {
				t.Helper()

				require.NoError(t, os.Chmod(filepath.Dir(parentDir), 0o777))
			},
			check:   assertNoDir,
			wantErr: true,
		},
		{
			name: "parent writable by others with sticky bit",
			setup: func(t *testing.T, parentDir, _ string) {
				t.Helper()

				require.NoError(t, os.Chmod(parentDir, os.ModeSticky|0o777))
			},
			check: func(t *testing.T, dir string) {
				t.Helper()

				assert.DirExists(t, dir)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := privateTempDir(t)

			uid := os.Getuid() + tc.uidStep
			want := filepath.Join(tempDir, "terragrunt-"+strconv.Itoa(uid))

			if tc.setup != nil {
				tc.setup(t, tempDir, want)
			}

			v := venvtest.NewOSWithEmptyEnv().
				WithTempDir(func() string { return tempDir }).
				WithGetUID(func() int { return uid })

			got, err := util.EnsureTempDir(v)

			if tc.check != nil {
				tc.check(t, want)
			}

			if tc.wantErr {
				var unsafeErr util.UnsafeTempDirError

				require.ErrorAs(t, err, &unsafeErr)
				assert.Equal(t, want, unsafeErr.Path)
				assert.Empty(t, got)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestEnsureTempDirRejectsParentOwnedByAnotherUser(t *testing.T) {
	t.Parallel()

	if os.Getuid() == 0 {
		t.Skip("root owns every test directory and is trusted, so it cannot stand in for another user")
	}

	parentDir := privateTempDir(t)
	require.NoError(t, os.Chmod(parentDir, os.ModeSticky|0o777))

	uid := os.Getuid() + 1
	want := filepath.Join(parentDir, "terragrunt-"+strconv.Itoa(uid))

	got, err := util.EnsureTempDir(
		venvtest.NewOSWithEmptyEnv().
			WithTempDir(func() string { return parentDir }).
			WithGetUID(func() int { return uid }),
	)

	var unsafeErr util.UnsafeTempDirError

	require.ErrorAs(t, err, &unsafeErr)
	assert.Equal(t, want, unsafeErr.Path)
	assert.Contains(t, unsafeErr.Reason, parentDir+" is owned by user id "+strconv.Itoa(os.Getuid()))
	assert.Empty(t, got)
	assert.NoDirExists(t, want)
}

func TestEnsureTempDirCreatesMissingTempDir(t *testing.T) {
	t.Parallel()

	tempDir := filepath.Join(privateTempDir(t), "missing", "tmp")

	got, err := util.EnsureTempDir(
		venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tempDir }),
	)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDir, "terragrunt-"+strconv.Itoa(os.Getuid())), got)
	assert.DirExists(t, got)
}

func TestEnsureTempDirResolvesSymlinkedTempDir(t *testing.T) {
	t.Parallel()

	base := privateTempDir(t)
	realDir := filepath.Join(base, "real")
	link := filepath.Join(base, "link")

	require.NoError(t, os.Mkdir(realDir, 0o700))
	require.NoError(t, os.Symlink(realDir, link))

	got, err := util.EnsureTempDir(
		venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return link }),
	)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(realDir, "terragrunt-"+strconv.Itoa(os.Getuid())), got)
}

func TestEnsureTempDirResolvesRelativeTempDir(t *testing.T) {
	t.Parallel()

	base := privateTempDir(t)

	v := venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return "relative" })
	platform := *v.Platform
	platform.Getwd = func() (string, error) { return base, nil }
	v.Platform = &platform

	got, err := util.EnsureTempDir(v)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "relative", "terragrunt-"+strconv.Itoa(os.Getuid())), got)
}

func TestEnsureTempDirOnMemFS(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	got, err := util.EnsureTempDir(v)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(v.Platform.TempDir(), "terragrunt-"+strconv.Itoa(v.Platform.GetUID())), got)
	assert.True(t, vfs.IsDir(v.FS, got))
}

func assertNoDir(t *testing.T, dir string) {
	t.Helper()

	assert.NoDirExists(t, dir)
}

func privateTempDir(t *testing.T) string {
	t.Helper()

	dir := helpers.TmpDirWOSymlinks(t)
	require.NoError(t, os.Chmod(dir, 0o700))

	return dir
}
