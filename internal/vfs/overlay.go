package vfs

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/spf13/afero"
)

// NewOverlayFS returns a filesystem that reads from base and writes to layer.
//
// A path that layer does not have is answered by base. Creating a file, or
// writing to one that only base has, lands in layer: the base copy is read
// into layer first, so base is never written. Directories are listed as the
// union of both, with layer's entry winning a name both hold.
//
// Returns [fs.ErrPermission] from Remove, RemoveAll and Rename for a path
// that base has, whether or not layer has it too: base is read-only, and the
// overlay cannot record a deletion, so removing layer's copy would only
// uncover base's.
//
// Symlinks in base resolve as base resolves them. A symlink created through
// the overlay is recorded in layer and followed only when layer also holds
// its target.
//
// Locks are taken on layer. [IsOSFS] reports false for the overlay.
func NewOverlayFS(base, layer FS) FS {
	return &overlayFS{
		Fs:    afero.NewCopyOnWriteFs(overlayBase{FS: base}, layer),
		base:  base,
		layer: layer,
	}
}

// overlayFS replaces the afero copy-on-write methods that compare errors by
// identity, which only the mem backend's errors match. Remove, RemoveAll and
// Rename also refuse paths afero would act on; see [NewOverlayFS].
type overlayFS struct {
	afero.Fs
	base  FS
	layer FS
}

// overlayBase hands afero's copy-on-write filesystem a missing path as the
// [fs.ErrNotExist] it compares against. The OS backend reports the platform
// errno instead, and on Windows that is not ENOENT, so without this every
// create under a base directory fails there. Only afero sees the rewritten
// error: the overlay's own Stat asks base directly.
type overlayBase struct {
	FS
}

func (b overlayBase) Stat(name string) (os.FileInfo, error) {
	info, err := b.FS.Stat(name)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return nil, &os.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}

	return info, err
}

func (fsys *overlayFS) Stat(name string) (os.FileInfo, error) {
	info, err := fsys.layer.Stat(name)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return info, err
	}

	return fsys.base.Stat(name)
}

// LstatIfPossible describes name from layer, or from base when layer has no
// entry there. The bool is true when Lstat answered, so a symlink is described
// as itself, and false when the filesystem that answered fell back to Stat.
func (fsys *overlayFS) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	info, kind, err := lstatOrStat(fsys.layer, name)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		info, kind, err = lstatOrStat(fsys.base, name)
	}

	return info, kind == statOfLink, err
}

func (fsys *overlayFS) SymlinkIfPossible(oldname, newname string) error {
	return Symlink(fsys.layer, oldname, newname)
}

func (fsys *overlayFS) ReadlinkIfPossible(name string) (string, error) {
	if _, err := Lstat(fsys.layer, name); err == nil || !errors.Is(err, fs.ErrNotExist) {
		return Readlink(fsys.layer, name)
	}

	return Readlink(fsys.base, name)
}

func (fsys *overlayFS) Remove(name string) error {
	if fsys.inBase(name) {
		return &os.PathError{Op: "remove", Path: name, Err: fs.ErrPermission}
	}

	return fsys.layer.Remove(name)
}

func (fsys *overlayFS) RemoveAll(name string) error {
	if fsys.inBase(name) {
		return &os.PathError{Op: "removeall", Path: name, Err: fs.ErrPermission}
	}

	return fsys.layer.RemoveAll(name)
}

func (fsys *overlayFS) Rename(oldname, newname string) error {
	if fsys.inBase(oldname) {
		return &os.LinkError{Op: "rename", Old: oldname, New: newname, Err: fs.ErrPermission}
	}

	return fsys.layer.Rename(oldname, newname)
}

func (fsys *overlayFS) Lock(name string) (Unlocker, error) {
	return Lock(fsys.layer, name)
}

func (fsys *overlayFS) TryLock(name string) (Unlocker, bool, error) {
	return TryLock(fsys.layer, name)
}

func (fsys *overlayFS) LockContext(ctx context.Context, name string) (Unlocker, error) {
	return LockContext(ctx, fsys.layer, name)
}

// inBase reports whether base has an entry at name, counting a dangling
// symlink, which a removal would otherwise reach.
func (fsys *overlayFS) inBase(name string) bool {
	_, err := Lstat(fsys.base, name)

	return err == nil
}
