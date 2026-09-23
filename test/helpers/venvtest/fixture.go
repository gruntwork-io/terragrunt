package venvtest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"

	"github.com/stretchr/testify/require"
)

const fixtureFileMode = 0o644

// Root returns a slash-rooted fixture path as the absolute path the host
// resolves it to. On Unix that is the path itself; on Windows "/repo" alone is
// not absolute, so it takes the volume of the working directory ("D:\\repo"),
// which is also where production code lands when it runs filepath.Abs on the
// same value. Every in-memory tree is rooted through it so the two agree.
func Root(path string) string {
	if !strings.HasPrefix(path, "/") {
		panic("venvtest.Root: path must be slash-rooted: " + path)
	}

	abs, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		panic("venvtest.Root: " + err.Error())
	}

	return abs
}

// Bounds for a fixture tree, which is small by construction; a test that trips
// these is pointing at the wrong directory.
const (
	fixtureMaxFiles = 5000
	fixtureMaxBytes = 64 << 20
)

// NewFS returns an in-memory filesystem holding files, each path taken
// relative to root. Pair it with WithFS to give a command a tree to walk
// without staging one on disk.
func NewFS(t *testing.T, root string, files map[string]string) vfs.FS {
	t.Helper()

	fsys := vfs.NewMemMapFS()

	for path, contents := range files {
		require.NoError(
			t,
			vfs.WriteFile(fsys, filepath.Join(root, path), []byte(contents), fixtureFileMode),
		)
	}

	return fsys
}

// LoadFS mirrors the on-disk tree at dir into an in-memory filesystem and
// returns it with the root the copy landed at, for fixtures that are easier to
// keep as files than as literals.
//
// The copy keeps the paths the tree has, so the returned root is dir itself. A
// subject that writes through os instead of the venv therefore leaves the copy
// untouched, and every assertion against it fails; a subject that only reads
// that way still finds the fixture, since it is really there.
func LoadFS(t *testing.T, dir string) (vfs.FS, string) {
	t.Helper()

	abs, err := filepath.Abs(dir)
	require.NoError(t, err)

	fsys, err := vfs.MirrorToMem(vfs.NewOSFS(), abs, vfs.MirrorLimits{
		MaxFiles: fixtureMaxFiles,
		MaxBytes: fixtureMaxBytes,
	})
	require.NoError(t, err)

	return fsys, abs
}
