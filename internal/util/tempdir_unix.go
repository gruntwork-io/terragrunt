//go:build !windows

package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

const (
	privateDirMode = os.FileMode(0o700)
	groupWritable  = os.FileMode(0o020)
	worldWritable  = os.FileMode(0o002)
)

func ensureTempDir(v *venv.Venv) (string, error) {
	v.RequireGetUID()
	v.RequireGetGID()

	uid := v.Platform.GetUID()
	gid := v.Platform.GetGID()

	parentDir, err := resolveTempDir(v)
	if err != nil {
		return "", err
	}

	tempDir := filepath.Join(parentDir, "terragrunt-"+strconv.Itoa(uid))

	if !vfs.IsOSFS(v.FS) {
		if err := v.FS.MkdirAll(tempDir, privateDirMode); err != nil {
			return "", err
		}

		return tempDir, nil
	}

	if err := checkParentDirs(v.FS, tempDir, gid); err != nil {
		return "", err
	}

	if err := v.FS.Mkdir(tempDir, privateDirMode); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}

	if err := checkPrivateDir(v.FS, tempDir, uid, gid); err != nil {
		return "", err
	}

	return tempDir, nil
}

func resolveTempDir(v *venv.Venv) (string, error) {
	dir := v.Platform.TempDir()

	if !filepath.IsAbs(dir) {
		wd, err := v.Platform.Getwd()
		if err != nil {
			return "", err
		}

		dir = filepath.Join(wd, dir)
	}

	if err := v.FS.MkdirAll(dir, privateDirMode); err != nil {
		return "", err
	}

	return vfs.EvalSymlinks(v.FS, dir)
}

func checkParentDirs(fsys vfs.FS, path string, gid int) error {
	dir := filepath.Dir(path)

	for range strings.Count(dir, string(filepath.Separator)) + 1 {
		info, err := vfs.Lstat(fsys, dir)
		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSticky == 0 && writableByOthers(info, gid) {
			return UnsafeTempDirError{
				Path:   path,
				Reason: fmt.Sprintf("other users can write to %s and it has no sticky bit (mode %#o)", dir, info.Mode().Perm()),
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}

		dir = parent
	}

	return nil
}

func checkPrivateDir(fsys vfs.FS, path string, uid, gid int) error {
	info, err := vfs.Lstat(fsys, path)
	if err != nil {
		return err
	}

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

	if writableByOthers(info, gid) {
		return UnsafeTempDirError{
			Path:   path,
			Reason: fmt.Sprintf("other users can write to it (mode %#o)", info.Mode().Perm()),
		}
	}

	return nil
}

func writableByOthers(info os.FileInfo, gid int) bool {
	perm := info.Mode().Perm()
	if perm&worldWritable != 0 {
		return true
	}

	stat, ok := info.Sys().(*syscall.Stat_t)

	return perm&groupWritable != 0 && (!ok || int64(stat.Gid) != int64(gid))
}
