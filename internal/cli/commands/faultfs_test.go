package commands_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// errInjected is the error every fault returns.
var errInjected = errors.New("injected fault")

// fault fails one filesystem operation on every path it matches.
type fault struct {
	match func(name string) bool
	op    string
}

// faultFS fails each call a fault matches with errInjected, including writes and closes of files it opens.
type faultFS struct {
	vfs.FS
	faults []fault
}

// faultFile fails Write or Close with errInjected.
type faultFile struct {
	vfs.File
	failWrite bool
	failClose bool
}

func newFaultFS(base vfs.FS, faults ...fault) *faultFS {
	return &faultFS{FS: base, faults: faults}
}

// atPath matches exactly path.
func atPath(op, path string) fault {
	return fault{op: op, match: func(name string) bool { return filepath.Clean(name) == filepath.Clean(path) }}
}

// atBasePrefix matches any path whose base name starts with prefix.
func atBasePrefix(op, prefix string) fault {
	return fault{op: op, match: func(name string) bool { return strings.HasPrefix(filepath.Base(name), prefix) }}
}

func (f *faultFS) Open(name string) (vfs.File, error) {
	if err := f.fail("open", name); err != nil {
		return nil, err
	}

	return f.FS.Open(name)
}

func (f *faultFS) OpenFile(name string, flag int, perm fs.FileMode) (vfs.File, error) {
	if err := f.fail("openfile", name); err != nil {
		return nil, err
	}

	file, err := f.FS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}

	return &faultFile{
		File:      file,
		failWrite: f.fail("write", name) != nil,
		failClose: f.fail("close", name) != nil,
	}, nil
}

func (f *faultFS) Mkdir(name string, perm fs.FileMode) error {
	if err := f.fail("mkdir", name); err != nil {
		return err
	}

	return f.FS.Mkdir(name, perm)
}

func (f *faultFS) MkdirAll(name string, perm fs.FileMode) error {
	if err := f.fail("mkdirall", name); err != nil {
		return err
	}

	return f.FS.MkdirAll(name, perm)
}

func (f *faultFS) Chmod(name string, mode fs.FileMode) error {
	if err := f.fail("chmod", name); err != nil {
		return err
	}

	return f.FS.Chmod(name, mode)
}

func (f *faultFS) Remove(name string) error {
	if err := f.fail("remove", name); err != nil {
		return err
	}

	return f.FS.Remove(name)
}

func (f *faultFS) RemoveAll(name string) error {
	if err := f.fail("removeall", name); err != nil {
		return err
	}

	return f.FS.RemoveAll(name)
}

func (f *faultFS) fail(op, name string) error {
	for _, flt := range f.faults {
		if flt.op == op && flt.match(name) {
			return &fs.PathError{Op: op, Path: name, Err: errInjected}
		}
	}

	return nil
}

func (f *faultFile) Write(p []byte) (int, error) {
	if f.failWrite {
		return 0, errInjected
	}

	return f.File.Write(p)
}

func (f *faultFile) Close() error {
	if err := f.File.Close(); err != nil {
		return err
	}

	if f.failClose {
		return errInjected
	}

	return nil
}
