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
