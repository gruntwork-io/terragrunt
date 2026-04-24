package glob_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{
			name:    "star matches within a single segment",
			pattern: "foo/*.tf",
			path:    "foo/bar.tf",
			want:    true,
		},
		{
			name:    "star does not cross a separator",
			pattern: "foo/*.tf",
			path:    "foo/bar/baz.tf",
			want:    false,
		},
		{
			name:    "double-star matches across segments",
			pattern: "foo/**/bar.tf",
			path:    "foo/a/b/bar.tf",
			want:    true,
		},
		{
			name:    "double-star collapses when flanking segments are literals",
			pattern: "foo/**/bar.tf",
			path:    "foo/bar.tf",
			want:    true,
		},
		{
			name:    "double-star does not collapse when trailing segment has a wildcard",
			pattern: "foo/**/*.tf",
			path:    "foo/root.tf",
			want:    false,
		},
		{
			name:    "double-star does not collapse when leading segment has a wildcard",
			pattern: "*/**/bar.tf",
			path:    "foo/bar.tf",
			want:    false,
		},
		{
			name:    "brace alternation is the reliable way to get zero-or-more depth with wildcards",
			pattern: "foo/{*.tf,**/*.tf}",
			path:    "foo/bar.tf",
			want:    true,
		},
		{
			name:    "question mark matches any single non-separator character",
			pattern: "f?o",
			path:    "foo",
			want:    true,
		},
		{
			name:    "question mark does not match the separator",
			pattern: "f?o",
			path:    "f/o",
			want:    false,
		},
		{
			name:    "character class matches listed alternatives",
			pattern: "[abc].tf",
			path:    "b.tf",
			want:    true,
		},
		{
			name:    "character class rejects outside the class",
			pattern: "[abc].tf",
			path:    "d.tf",
			want:    false,
		},
		{
			name:    "backslash escapes a metacharacter to match literally",
			pattern: `a\*b.tf`,
			path:    "a*b.tf",
			want:    true,
		},
		{
			name:    "escaped metacharacter does not match as a wildcard",
			pattern: `a\*b.tf`,
			path:    "acb.tf",
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			matcher, err := glob.Compile(tc.pattern)
			require.NoError(t, err)

			assert.Equal(t, tc.want, matcher.Match(tc.path))
		})
	}
}

func TestCompileRejectsInvalidPattern(t *testing.T) {
	t.Parallel()

	// An unterminated character class is rejected by the underlying matcher.
	_, err := glob.Compile("[unterminated")
	require.Error(t, err)
}

func TestExpand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   []string
		dirs    []string
		pattern string
		opts    []glob.ExpandOption
		want    []string
	}{
		{
			name:    "static path hits an existing file",
			files:   []string{"/mod/main.tf"},
			pattern: "/mod/main.tf",
			want:    []string{"/mod/main.tf"},
		},
		{
			name:    "static path returns empty when the file is missing",
			files:   []string{"/mod/main.tf"},
			pattern: "/mod/absent.tf",
			want:    nil,
		},
		{
			name:    "static path returns the directory when no files-only",
			files:   []string{"/mod/keep.txt"},
			dirs:    []string{"/mod/sub"},
			pattern: "/mod/sub",
			want:    []string{"/mod/sub"},
		},
		{
			name:    "static path with files-only drops a directory match",
			files:   []string{"/mod/keep.txt"},
			dirs:    []string{"/mod/sub"},
			pattern: "/mod/sub",
			opts:    []glob.ExpandOption{glob.WithFilesOnly()},
			want:    nil,
		},
		{
			name: "star matches files in one directory",
			files: []string{
				"/mod/a.tf",
				"/mod/b.tf",
				"/mod/nested/c.tf",
				"/mod/README.md",
			},
			pattern: "/mod/*.tf",
			want:    []string{"/mod/a.tf", "/mod/b.tf"},
		},
		{
			name: "double-star matches nested files but not the root sibling",
			files: []string{
				"/mod/root.tf",
				"/mod/sub/a.tf",
				"/mod/sub/deep/b.tf",
			},
			pattern: "/mod/**/*.tf",
			want:    []string{"/mod/sub/a.tf", "/mod/sub/deep/b.tf"},
		},
		{
			name: "brace alternation covers root and nested files",
			files: []string{
				"/mod/root.tf",
				"/mod/sub/nested.tf",
			},
			pattern: "/mod/{*.tf,**/*.tf}",
			want:    []string{"/mod/root.tf", "/mod/sub/nested.tf"},
		},
		{
			name: "files-only skips directories matched by the pattern",
			files: []string{
				"/mod/keep.tf",
			},
			dirs:    []string{"/mod/also"},
			pattern: "/mod/*",
			opts:    []glob.ExpandOption{glob.WithFilesOnly()},
			want:    []string{"/mod/keep.tf"},
		},
		{
			name: "without files-only the same pattern includes the directory",
			files: []string{
				"/mod/keep.tf",
			},
			dirs:    []string{"/mod/also"},
			pattern: "/mod/*",
			want:    []string{"/mod/also", "/mod/keep.tf"},
		},
		{
			name: "backslash escape matches a literal star in a filename",
			files: []string{
				"/mod/a*b.tf",
				"/mod/acb.tf",
			},
			pattern: `/mod/a\*b.tf`,
			want:    []string{"/mod/a*b.tf"},
		},
		{
			name:    "missing root directory returns empty without error",
			files:   nil,
			pattern: "/does-not-exist/**/*.tf",
			want:    nil,
		},
		{
			name: "pattern that matches nothing returns empty",
			files: []string{
				"/mod/README.md",
			},
			pattern: "/mod/*.tf",
			want:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()
			for _, p := range tc.files {
				require.NoError(t, vfs.WriteFile(fs, p, []byte{}, 0o644))
			}

			for _, d := range tc.dirs {
				require.NoError(t, fs.MkdirAll(d, 0o755))
			}

			got, err := glob.Expand(fs, tc.pattern, tc.opts...)
			require.NoError(t, err)

			// Sort both sides so the assertion is robust to walker ordering
			// changes without asserting a specific traversal order.
			slices.Sort(got)

			want := slices.Clone(tc.want)
			slices.Sort(want)

			assert.Equal(t, want, got, "pattern %q", tc.pattern)
		})
	}
}

func TestExpandRejectsInvalidPattern(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/mod/a.tf", []byte{}, 0o644))

	_, err := glob.Expand(fs, "/mod/[unterminated")
	require.Error(t, err)
}

// TestLegacyExpandCollapsesGlobstar documents the reason LegacyExpand exists:
// zglob collapses the separators flanking `**`, whereas Expand (gobwas) does
// not. Switching the include_in_copy / exclude_from_copy call site from
// LegacyExpand to Expand would silently change user-facing behavior for
// patterns like `foo/**/bar.tf`.
//
// LegacyExpand reaches the real filesystem, so this test uses t.TempDir()
// instead of MemMapFS.
func TestLegacyExpandCollapsesGlobstar(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "root.tf"), nil, 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "nested.tf"), nil, 0o644))

	pattern := filepath.ToSlash(root) + "/**/*.tf"

	got, err := glob.LegacyExpand(pattern)
	require.NoError(t, err)

	slices.Sort(got)

	want := []string{
		filepath.Join(root, "root.tf"),
		filepath.Join(root, "sub", "nested.tf"),
	}
	slices.Sort(want)

	assert.Equal(t, want, got,
		"zglob collapses `**` so root.tf matches even though it sits alongside sub/")
}

// TestExpandDivergesFromLegacyOnGlobstar is the counterpart: same pattern
// shape, same tree, different result under gobwas semantics. Keeps the
// divergence visible so a future refactor cannot quietly erase it.
func TestExpandDivergesFromLegacyOnGlobstar(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/mod/root.tf", nil, 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/mod/sub/nested.tf", nil, 0o644))

	got, err := glob.Expand(fs, "/mod/**/*.tf")
	require.NoError(t, err)

	// Only the nested file survives: gobwas does not collapse `**`.
	assert.Equal(t, []string{"/mod/sub/nested.tf"}, got)
}
