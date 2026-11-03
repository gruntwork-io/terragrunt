//go:build !windows

package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

const (
	// privateDirMode lets only the owner create, rename, or remove entries.
	privateDirMode = os.FileMode(0o700)

	// groupOrOtherWrite is the permission bits that let other users plant entries.
	groupOrOtherWrite = os.FileMode(0o022)
)

// ensureTempDir uses TMPDIR/terragrunt-<uid>. Another user can create that
// path first, so a directory already there is checked before it is used.
func ensureTempDir(v *venv.Venv) (string, error) {
	v.RequireGetUID()

	uid := v.Platform.GetUID()
	parentDir := v.Platform.TempDir()
	tempDir := filepath.Join(parentDir, "terragrunt-"+strconv.Itoa(uid))

	// A TMPDIR that does not exist yet is created, as before the directory was per user.
	if err := v.FS.MkdirAll(parentDir, os.ModePerm); err != nil {
		return "", err
	}

	// Mkdir rather than MkdirAll: it fails on a symlink at the path instead of following it.
	if err := v.FS.Mkdir(tempDir, privateDirMode); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}

	info, err := vfs.Lstat(v.FS, tempDir)
	if err != nil {
		return "", err
	}

	if err := checkPrivateDir(tempDir, info, uid); err != nil {
		return "", err
	}

	return tempDir, nil
}

// checkPrivateDir returns [UnsafeTempDirError] unless info describes a
// directory owned by uid that no other user can write to.
func checkPrivateDir(path string, info os.FileInfo, uid int) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return UnsafeTempDirError{Path: path, Reason: "it is a symlink"}
	}

	if !info.IsDir() {
		return UnsafeTempDirError{Path: path, Reason: "it is not a directory"}
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return UnsafeTempDirError{Path: path, Reason: "its owner cannot be read"}
	}

	if int64(stat.Uid) != int64(uid) {
		return UnsafeTempDirError{
			Path:   path,
			Reason: fmt.Sprintf("it is owned by user id %d, not %d", stat.Uid, uid),
		}
	}

	if info.Mode().Perm()&groupOrOtherWrite != 0 {
		return UnsafeTempDirError{
			Path:   path,
			Reason: fmt.Sprintf("other users can write to it (mode %s)", info.Mode().Perm()),
		}
	}

	return nil
}
