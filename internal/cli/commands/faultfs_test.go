package commands_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"

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
	faults   []fault
	injected []injection
	mu       sync.Mutex
}

// faultFile fails Write or Close with errInjected.
type faultFile struct {
	vfs.File
	fsys *faultFS
	name string
}

type injection struct {
	op   string
	path string
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

	return &faultFile{File: file, fsys: f, name: name}, nil
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

// injections returns the operations f failed, in order.
func (f *faultFS) injections() []injection {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.injected)
}

func (f *faultFS) fail(op, name string) error {
	for _, flt := range f.faults {
		if flt.op == op && flt.match(name) {
			f.mu.Lock()
			f.injected = append(f.injected, injection{op: op, path: name})
			f.mu.Unlock()

			return &fs.PathError{Op: op, Path: name, Err: errInjected}
		}
	}

	return nil
}

func (f *faultFile) Write(p []byte) (int, error) {
	if f.fsys.fail("write", f.name) != nil {
		return 0, errInjected
	}

	return f.File.Write(p)
}

func (f *faultFile) Close() error {
	if err := f.File.Close(); err != nil {
		return err
	}

	if f.fsys.fail("close", f.name) != nil {
		return errInjected
	}

	return nil
}
