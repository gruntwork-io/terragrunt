//go:build !windows

package util_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
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
		setup      func(t *testing.T, parentDir, dir string)
		check      func(t *testing.T, dir string)
		name       string
		wantReason string
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
			wantReason: "it is writable by its group or by others (mode 0770)",
		},
		{
			name: "world writable with sticky bit",
			setup: func(t *testing.T, _, dir string) {
				t.Helper()

				require.NoError(t, os.Mkdir(dir, 0o700))
				require.NoError(t, os.Chmod(dir, os.ModeSticky|0o777))
			},
			wantReason: "it is writable by its group or by others (mode 0777)",
		},
		{
			name: "symlink to a private directory",
			setup: func(t *testing.T, _, dir string) {
				t.Helper()

				target := filepath.Join(filepath.Dir(dir), "target")
				require.NoError(t, os.Mkdir(target, 0o700))
				require.NoError(t, os.Symlink(target, dir))
			},
			wantReason: "it is a symlink",
		},
		{
			name: "dangling symlink",
			setup: func(t *testing.T, _, dir string) {
				t.Helper()

				require.NoError(t, os.Symlink(filepath.Join(filepath.Dir(dir), "missing"), dir))
			},
			wantReason: "it is a symlink",
		},
		{
			name: "regular file",
			setup: func(t *testing.T, _, dir string) {
				t.Helper()

				require.NoError(t, os.WriteFile(dir, nil, 0o600))
			},
			wantReason: "it is not a directory",
		},
		{
			name: "parent writable by others without sticky bit",
			setup: func(t *testing.T, parentDir, _ string) {
				t.Helper()

				require.NoError(t, os.Chmod(parentDir, 0o777))
			},
			check:      assertNoDir,
			wantReason: "is writable by its group or by others and has no sticky bit (mode 0777)",
		},
		{
			name: "parent writable by group without sticky bit",
			setup: func(t *testing.T, parentDir, _ string) {
				t.Helper()

				require.NoError(t, os.Chmod(parentDir, 0o770))
			},
			check:      assertNoDir,
			wantReason: "is writable by its group or by others and has no sticky bit (mode 0770)",
		},
		{
			name: "grandparent writable by others without sticky bit",
			setup: func(t *testing.T, parentDir, _ string) {
				t.Helper()

				require.NoError(t, os.Chmod(filepath.Dir(parentDir), 0o777))
			},
			check:      assertNoDir,
			wantReason: "is writable by its group or by others and has no sticky bit (mode 0777)",
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

			want := filepath.Join(tempDir, "terragrunt-"+strconv.Itoa(os.Getuid()))

			if tc.setup != nil {
				tc.setup(t, tempDir, want)
			}

			got, err := util.EnsureTempDir(
				venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tempDir }),
			)

			if tc.check != nil {
				tc.check(t, want)
			}

			if tc.wantReason != "" {
				var unsafeErr util.UnsafeTempDirError

				require.ErrorAs(t, err, &unsafeErr)
				assert.Equal(t, want, unsafeErr.Path)
				assert.Contains(t, unsafeErr.Reason, tc.wantReason)
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

func TestEnsureTempDirRejectsDirOwnedByAnotherUser(t *testing.T) {
	t.Parallel()

	parentDir, err := filepath.EvalSymlinks("/tmp")
	require.NoError(t, err)

	info, err := os.Stat(parentDir)
	require.NoError(t, err)

	stat, ok := info.Sys().(*syscall.Stat_t)
	if os.Getuid() == 0 || !ok || stat.Uid != 0 || info.Mode()&os.ModeSticky == 0 {
		t.Skip("needs a non-root user and a root-owned /tmp with the sticky bit")
	}

	unusedUID := 0
	want := ""

	for offset := range 100 {
		uid := 1_000_000_000 + os.Getpid() + offset
		dir := filepath.Join(parentDir, "terragrunt-"+strconv.Itoa(uid))

		err := os.Mkdir(dir, 0o700)
		if errors.Is(err, os.ErrExist) {
			continue
		}

		require.NoError(t, err)
		t.Cleanup(func() {
			assert.NoError(t, os.Remove(dir))
		})

		unusedUID, want = uid, dir

		break
	}

	require.NotEmpty(t, want, "every candidate directory name in %s is taken", parentDir)

	got, err := util.EnsureTempDir(
		venvtest.NewOSWithEmptyEnv().
			WithTempDir(func() string { return parentDir }).
			WithGetUID(func() int { return unusedUID }),
	)

	var unsafeErr util.UnsafeTempDirError

	require.ErrorAs(t, err, &unsafeErr)
	assert.Equal(t, want, unsafeErr.Path)
	assert.Equal(t, "it is owned by user id "+strconv.Itoa(os.Getuid())+", not "+strconv.Itoa(unusedUID), unsafeErr.Reason)
	assert.Empty(t, got)
}

func TestEnsureTempDirRejectsParentOwnedByMappedUID65534(t *testing.T) {
	t.Parallel()

	if os.Getuid() != 0 {
		t.Skip("only root can give a directory to user id 65534")
	}

	parentDir := privateTempDir(t)
	require.NoError(t, os.Chmod(parentDir, os.ModeSticky|0o777))

	if err := os.Chown(parentDir, 65534, 65534); err != nil {
		t.Skipf("user id 65534 is not mapped in this user namespace: %v", err)
	}

	got, err := util.EnsureTempDir(venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return parentDir }))

	var unsafeErr util.UnsafeTempDirError

	require.ErrorAs(t, err, &unsafeErr)
	assert.Equal(t, parentDir+" is owned by user id 65534, who can replace anything in it", unsafeErr.Reason)
	assert.Empty(t, got)
	assertNoDir(t, filepath.Join(parentDir, "terragrunt-0"))
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
