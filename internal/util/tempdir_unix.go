//go:build !windows

package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

const (
	privateDirMode = os.FileMode(0o700)
	othersWritable = os.FileMode(0o022)
	rootUID        = 0
	// The kernel shows owners outside the current user namespace, such as host root, as this user ID.
	overflowUID = 65534
	uidMapPath  = "/proc/self/uid_map"
)

// TempDirTrustedOwners returns uid and root, plus the overflow user id when the uid map leaves it unmapped.
func TempDirTrustedOwners(fsys vfs.FS, uid int) []int64 {
	owners := []int64{int64(uid), rootUID}

	uidMap, err := vfs.ReadFile(fsys, uidMapPath)
	if err != nil {
		return owners
	}

	for line := range strings.Lines(string(uidMap)) {
		var inside, outside, count int64

		if _, err := fmt.Sscan(line, &inside, &outside, &count); err != nil {
			return owners
		}

		if inside <= overflowUID && overflowUID < inside+count {
			return owners
		}
	}

	return []int64{int64(uid), rootUID, overflowUID}
}

func ensureTempDir(v *venv.Venv) (string, error) {
	v.RequireGetUID()

	uid := v.Platform.GetUID()

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

	if err := checkParentDirs(v.FS, tempDir, TempDirTrustedOwners(v.FS, uid)); err != nil {
		return "", err
	}

	if err := v.FS.Mkdir(tempDir, privateDirMode); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}

	if err := checkPrivateDir(v.FS, tempDir, uid); err != nil {
		return "", err
	}

	return tempDir, nil
}

func resolveTempDir(v *venv.Venv) (string, error) {
	dir := v.Platform.TempDir()

	if !filepath.IsAbs(dir) {
		v.RequireGetwd()

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

func checkParentDirs(fsys vfs.FS, path string, trusted []int64) error {
	dir := filepath.Dir(path)

	for range strings.Count(dir, string(filepath.Separator)) + 1 {
		info, err := vfs.Lstat(fsys, dir)
		if err != nil {
			return err
		}

		owner, err := ownerOf(path, dir, info)
		if err != nil {
			return err
		}

		if !slices.Contains(trusted, owner) {
			return UnsafeTempDirError{
				Path:   path,
				Reason: fmt.Sprintf("%s is owned by user id %d, who can replace anything in it", dir, owner),
			}
		}

		if info.Mode()&os.ModeSticky == 0 && info.Mode().Perm()&othersWritable != 0 {
			return UnsafeTempDirError{
				Path: path,
				Reason: fmt.Sprintf(
					"%s is writable by its group or by others and has no sticky bit (mode %#o)",
					dir,
					info.Mode().Perm(),
				),
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

func checkPrivateDir(fsys vfs.FS, path string, uid int) error {
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

	owner, err := ownerOf(path, path, info)
	if err != nil {
		return err
	}

	if owner != int64(uid) {
		return UnsafeTempDirError{
			Path:   path,
			Reason: fmt.Sprintf("it is owned by user id %d, not %d", owner, uid),
		}
	}

	if info.Mode().Perm()&othersWritable != 0 {
		return UnsafeTempDirError{
			Path:   path,
			Reason: fmt.Sprintf("it is writable by its group or by others (mode %#o)", info.Mode().Perm()),
		}
	}

	return nil
}

func ownerOf(path, dir string, info os.FileInfo) (int64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, UnsafeTempDirError{Path: path, Reason: "the owner of " + dir + " cannot be read"}
	}

	return int64(stat.Uid), nil
}
