package glob_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/mattn/go-zglob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyTree is the shape LegacyExpand is exercised against: nested
// directories, several extensions, a dotfile, and a directory whose name is a
// prefix of another so that a partial match cannot pass by accident.
var legacyTree = []string{
	"main.tf",
	"README.md",
	".hidden.tf",
	"modules/a/main.tf",
	"modules/a/vars.tf",
	"modules/a/nested/deep.tf",
	"modules/ab/main.tf",
	"modules/b/main.tfvars",
	"vendor/x/main.tf",
}

// legacyPatterns covers the grammar zglob accepts, including the "**"
// collapsing that distinguishes it from [glob.Expand].
var legacyPatterns = []string{
	"*",
	"*.tf",
	"*.md",
	"modules",
	"modules/*",
	"modules/*/main.tf",
	"modules/**/*.tf",
	"modules/**",
	"**/*.tf",
	"**/main.tf",
	"modules/a/*",
	"modules/a/**/*.tf",
	"modules/{a,b}/*",
	"modules/a?/main.tf",
	"modules/[ab]/main.tf",
	"vendor/**/*.tf",
	"nothing-here/*",
	"main.tf",
	"missing.tf",
	"modules/a/nested/deep.tf",
}

// TestLegacyExpandMatchesZglob pins LegacyExpand to the grammar it exists to
// preserve: for every pattern in the corpus, walking the venv filesystem
// returns exactly what zglob's own walk returns over the same tree on disk.
//
// The tree is written to disk precisely so the comparison is against real
// zglob rather than against a second copy of our own logic.
func TestLegacyExpandMatchesZglob(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// macOS puts temp dirs behind a symlink, and zglob reports the path it
	// walked rather than the one it was handed, so both sides have to start
	// from the resolved spelling to be comparable.
	root, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)

	for _, rel := range legacyTree {
		full := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(rel), 0o600))
	}

	fsys := vfs.NewOSFS()

	for _, pattern := range legacyPatterns {
		absolute := filepath.ToSlash(filepath.Join(root, pattern))

		want, wantErr := zglob.Glob(absolute)
		got, gotErr := glob.LegacyExpand(fsys, absolute)

		if wantErr != nil {
			require.Error(t, gotErr, "pattern %q: zglob failed but LegacyExpand did not", pattern)
			require.ErrorIs(t, gotErr, os.ErrNotExist, "pattern %q", pattern)

			continue
		}

		require.NoError(t, gotErr, "pattern %q", pattern)
		assert.ElementsMatch(t, want, got, "pattern %q", pattern)
	}
}

// TestLegacyExpandReportsMissingLiteralPath pins the error the caller in
// internal/util depends on: a literal path that does not exist is reported as
// fs.ErrNotExist, not as an empty match, because expandGlobPath treats those
// two differently.
func TestLegacyExpandReportsMissingLiteralPath(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/src/main.tf", nil, 0o644))

	got, err := glob.LegacyExpand(fsys, "/src/main.tf")
	require.NoError(t, err)
	assert.Equal(t, []string{"/src/main.tf"}, got)

	_, err = glob.LegacyExpand(fsys, "/src/missing.tf")
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestLegacyExpandRunsOnAnInMemoryFilesystem pins that the expansion resolves
// entirely against the filesystem it is handed, never the real disk.
func TestLegacyExpandRunsOnAnInMemoryFilesystem(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	for _, p := range []string{"/src/main.tf", "/src/modules/a/main.tf", "/src/README.md"} {
		require.NoError(t, vfs.WriteFile(fsys, p, nil, 0o644))
	}

	got, err := glob.LegacyExpand(fsys, "/src/**/*.tf")
	require.NoError(t, err)

	// zglob collapses `**`, so the top-level file matches alongside the nested one.
	assert.ElementsMatch(t, []string{"/src/main.tf", "/src/modules/a/main.tf"}, got)
}

// TestLegacyExpandDoesNotExpandEnvSegments pins the one place LegacyExpand
// parts company with zglob. zglob expands a "$NAME" segment from the process
// environment when choosing its walk root; the walk here roots at a literal
// "$NAME" directory instead and reports it missing, which the caller in
// internal/util reads as no matches.
func TestLegacyExpandDoesNotExpandEnvSegments(t *testing.T) {
	t.Setenv("LEGACY_EXPAND_PROBE", "real")

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/src/real/main.tf", nil, 0o644))

	_, err := glob.LegacyExpand(fsys, "/src/$LEGACY_EXPAND_PROBE/*.tf")
	require.ErrorIs(t, err, fs.ErrNotExist)

	// The same tree is reachable when the segment is spelled literally.
	got, err := glob.LegacyExpand(fsys, "/src/real/*.tf")
	require.NoError(t, err)
	assert.Equal(t, []string{"/src/real/main.tf"}, got)
}

// symlinkedRootPatterns are walk roots that pass through a symlinked
// directory, the shape expandGlobPath produces for a symlinked
// include_in_copy entry (issue #6791).
var symlinkedRootPatterns = []string{
	"linked",
	"linked/*",
	"linked/**",
	"linked/**/*.tf",
	"linked/nested/*",
	"linked-rel/*",
	"linked-rel/**/*.tf",
}

// TestLegacyExpandMatchesZglobOnSymlinkedRoot pins that, under
// [glob.WithSymlinkedRoots], a pattern rooted at a symlinked directory expands
// through the link exactly as zglob does, since zglob stats its walk root
// through symlinks (issue #6791).
func TestLegacyExpandMatchesZglobOnSymlinkedRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	root, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)

	target := filepath.Join(root, "target")
	for _, rel := range []string{"main.tf", "vars.tf", "nested/deep.tf", "data.txt"} {
		full := filepath.Join(target, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(rel), 0o600))
	}

	if err := os.Symlink(target, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	// A link with a relative target must resolve against the link's own directory.
	require.NoError(t, os.Symlink("target", filepath.Join(root, "linked-rel")))

	fsys := vfs.NewOSFS()

	for _, pattern := range symlinkedRootPatterns {
		absolute := filepath.ToSlash(filepath.Join(root, pattern))

		want, wantErr := zglob.Glob(absolute)
		require.NoError(t, wantErr, "pattern %q", pattern)

		got, gotErr := glob.LegacyExpand(fsys, absolute, glob.WithSymlinkedRoots())
		require.NoError(t, gotErr, "pattern %q", pattern)
		assert.ElementsMatch(t, want, got, "pattern %q", pattern)
	}
}

// TestLegacyExpandExpandsThroughSymlinkedRoot pins the same symlinked-root
// expansion on the in-memory filesystem, where symlinks live in a side table,
// and that without [glob.WithSymlinkedRoots] the link stays opaque.
func TestLegacyExpandExpandsThroughSymlinkedRoot(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, fsys.MkdirAll("/src", 0o755))
	require.NoError(t, vfs.WriteFile(fsys, "/target/stuff1", []byte("yay"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, "/target/stuff2", []byte("yay"), 0o644))
	require.NoError(t, vfs.Symlink(fsys, "/target", "/src/.important_stuff"))

	got, err := glob.LegacyExpand(fsys, "/src/.important_stuff/*")
	require.NoError(t, err)
	assert.Empty(t, got, "without the option the symlinked root must not expand")

	got, err = glob.LegacyExpand(fsys, "/src/.important_stuff/*", glob.WithSymlinkedRoots())
	require.NoError(t, err)

	// Matches are reported under the link's own spelling, not the target's.
	assert.ElementsMatch(
		t,
		[]string{"/src/.important_stuff/stuff1", "/src/.important_stuff/stuff2"},
		got,
	)
}

// TestLegacyExpandDanglingSymlinkRootMatchesZglob pins that a pattern rooted
// at a dangling symlink reports fs.ErrNotExist the way zglob does, which
// expandGlobPath in internal/util reads as no matches rather than a failure.
func TestLegacyExpandDanglingSymlinkRootMatchesZglob(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	root, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)

	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "dangling")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	pattern := filepath.ToSlash(filepath.Join(root, "dangling", "*"))

	_, wantErr := zglob.Glob(pattern)
	require.ErrorIs(t, wantErr, fs.ErrNotExist)

	_, gotErr := glob.LegacyExpand(vfs.NewOSFS(), pattern, glob.WithSymlinkedRoots())
	require.ErrorIs(t, gotErr, fs.ErrNotExist)
}

// TestLegacyExpandDanglingSymlinkRootOnMemFS pins the same not-exist report on
// the in-memory filesystem, simulating the broken-link failure without disk.
func TestLegacyExpandDanglingSymlinkRootOnMemFS(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, fsys.MkdirAll("/src", 0o755))
	require.NoError(t, vfs.Symlink(fsys, "/missing", "/src/dangling"))

	_, err := glob.LegacyExpand(fsys, "/src/dangling/*", glob.WithSymlinkedRoots())
	require.ErrorIs(t, err, fs.ErrNotExist)

	// Without the option the dangling link is opaque: no matches, no error.
	got, err := glob.LegacyExpand(fsys, "/src/dangling/*")
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestLegacyExpandSymlinkToFileRootMatchesZglob pins that a pattern rooted at
// a symlink resolving to a regular file expands to nothing, without error, on
// both sides.
func TestLegacyExpandSymlinkToFileRootMatchesZglob(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	root, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)

	file := filepath.Join(root, "plain.txt")
	require.NoError(t, os.WriteFile(file, []byte("plain"), 0o600))

	if err := os.Symlink(file, filepath.Join(root, "linkfile")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	pattern := filepath.ToSlash(filepath.Join(root, "linkfile", "*"))

	want, wantErr := zglob.Glob(pattern)
	require.NoError(t, wantErr)

	got, gotErr := glob.LegacyExpand(vfs.NewOSFS(), pattern, glob.WithSymlinkedRoots())
	require.NoError(t, gotErr)
	assert.ElementsMatch(t, want, got)
}
