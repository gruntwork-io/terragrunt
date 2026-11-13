package providercaching_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// errInjected is the error every fault returns.
var errInjected = errors.New("injected fault")

// faultFS fails the directory operation named op, on every path whose base name starts with basePrefix, with
// errInjected.
type faultFS struct {
	vfs.FS
	op         string
	basePrefix string
}

func (f *faultFS) Mkdir(name string, perm fs.FileMode) error {
	if f.fails("mkdir", name) {
		return errInjected
	}

	return f.FS.Mkdir(name, perm)
}

func (f *faultFS) MkdirAll(name string, perm fs.FileMode) error {
	if f.fails("mkdirall", name) {
		return errInjected
	}

	return f.FS.MkdirAll(name, perm)
}

func (f *faultFS) RemoveAll(name string) error {
	if f.fails("removeall", name) {
		return errInjected
	}

	return f.FS.RemoveAll(name)
}

func (f *faultFS) fails(op, name string) bool {
	return f.op == op && strings.HasPrefix(filepath.Base(name), f.basePrefix)
}
