package vfs_test

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mirrorTree is the fixture every mirror test copies, keyed by slash-separated
// path relative to the root. The cache, .terraform, and .git entries sit
// inside units, where a mirror that forgot to prune would pick them up.
var mirrorTree = map[string]string{
	"root.hcl":                         "root",
	"unit/terragrunt.hcl":              "unit",
	"unit/vars.tfvars":                 "vars",
	"unit/.terragrunt-cache/x/main.tf": "cache",
	"unit/.terraform/providers/p":      "provider",
	"stack/nested/deep/terragrunt.hcl": "deep",
	"stack/nested/deep/.git/HEAD":      "head",
	"stack/README.md":                  "readme",
}

// wanted is the subset of mirrorTree a mirror with no selector copies.
var wanted = []string{
	"root.hcl",
	"stack/README.md",
	"stack/nested/deep/terragrunt.hcl",
	"unit/terragrunt.hcl",
	"unit/vars.tfvars",
}

// hclOnly is the subset of wanted that selectHCL keeps.
var hclOnly = []string{
	"root.hcl",
	"stack/nested/deep/terragrunt.hcl",
	"unit/terragrunt.hcl",
}

// generousLimits admits the whole fixture.
var generousLimits = vfs.MirrorLimits{MaxFiles: 100, MaxBytes: 1 << 20}

// selectHCL is a selector that keeps files by extension.
func selectHCL(files []string) ([]string, error) {
	var kept []string

	for _, f := range files {
		if strings.HasSuffix(f, ".hcl") {
			kept = append(kept, f)
		}
	}

	return kept, nil
}

// writeMirrorTree stages mirrorTree under root on fsys.
func writeMirrorTree(t *testing.T, fsys vfs.FS, root string) {
	t.Helper()

	for rel, contents := range mirrorTree {
		require.NoError(
			t,
			vfs.WriteFile(
				fsys,
				filepath.Join(root, filepath.FromSlash(rel)),
				[]byte(contents),
				0o644,
			),
		)
	}
}

// mirroredPaths lists every file in fsys under root, relative to it with
// forward slashes, so a test can compare against mirrorTree's keys.
func mirroredPaths(t *testing.T, fsys vfs.FS, root string) []string {
	t.Helper()

	var paths []string

	require.NoError(t, vfs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		paths = append(paths, filepath.ToSlash(rel))

		return nil
	}))

	return paths
}

// bytesOf sums the fixture contents of the given relative paths.
func bytesOf(rels []string) int64 {
	var total int64
	for _, rel := range rels {
		total += int64(len(mirrorTree[rel]))
	}

	return total
}

func TestMirrorToMem(t *testing.T) {
	t.Parallel()

	t.Run("copies files at their own paths and prunes generated dirs", func(t *testing.T) {
		t.Parallel()

		const root = "/estate"

		src := vfs.NewMemMapFS()
		writeMirrorTree(t, src, root)

		dst, err := vfs.MirrorToMem(src, root, generousLimits)
		require.NoError(t, err)

		assert.ElementsMatch(t, wanted, mirroredPaths(t, dst, root))

		contents, err := vfs.ReadFileAsString(
			dst,
			filepath.Join(root, "stack", "nested", "deep", "terragrunt.hcl"),
		)
		require.NoError(t, err)
		assert.Equal(t, "deep", contents)
	})

	t.Run("leaves the source untouched", func(t *testing.T) {
		t.Parallel()

		const root = "/estate"

		src := vfs.NewMemMapFS()
		writeMirrorTree(t, src, root)

		dst, err := vfs.MirrorToMem(src, root, generousLimits)
		require.NoError(t, err)

		require.NoError(t, vfs.WriteFile(dst, filepath.Join(root, "new.hcl"), []byte("x"), 0o644))

		assert.False(t, vfs.Exists(src, filepath.Join(root, "new.hcl")))
	})

	t.Run("refuses a tree with more files than MaxFiles", func(t *testing.T) {
		t.Parallel()

		const root = "/estate"

		src := vfs.NewMemMapFS()
		writeMirrorTree(t, src, root)

		limits := generousLimits
		limits.MaxFiles = len(wanted) - 1

		_, err := vfs.MirrorToMem(src, root, limits)
		require.ErrorIs(t, err, vfs.ErrMirrorTooLarge)
	})

	t.Run("admits a tree with exactly MaxFiles files", func(t *testing.T) {
		t.Parallel()

		const root = "/estate"

		src := vfs.NewMemMapFS()
		writeMirrorTree(t, src, root)

		limits := generousLimits
		limits.MaxFiles = len(wanted)

		_, err := vfs.MirrorToMem(src, root, limits)
		require.NoError(t, err)
	})

	t.Run("refuses a tree with more bytes than MaxBytes", func(t *testing.T) {
		t.Parallel()

		const root = "/estate"

		src := vfs.NewMemMapFS()
		writeMirrorTree(t, src, root)

		limits := generousLimits
		limits.MaxBytes = bytesOf(wanted) - 1

		_, err := vfs.MirrorToMem(src, root, limits)
		require.ErrorIs(t, err, vfs.ErrMirrorTooLarge)
	})

	t.Run("copies only the files the selector returns", func(t *testing.T) {
		t.Parallel()

		const root = "/estate"

		src := vfs.NewMemMapFS()
		writeMirrorTree(t, src, root)

		dst, err := vfs.MirrorToMem(src, root, generousLimits, vfs.WithSelector(selectHCL))
		require.NoError(t, err)

		assert.ElementsMatch(t, hclOnly, mirroredPaths(t, dst, root))
	})

	t.Run("hands the selector every candidate, sorted, at its real path", func(t *testing.T) {
		t.Parallel()

		const root = "/estate"

		src := vfs.NewMemMapFS()
		writeMirrorTree(t, src, root)

		var offered []string

		_, err := vfs.MirrorToMem(
			src,
			root,
			generousLimits,
			vfs.WithSelector(func(files []string) ([]string, error) {
				offered = files

				return nil, nil
			}),
		)
		require.NoError(t, err)

		want := make([]string, 0, len(wanted))
		for _, rel := range wanted {
			want = append(want, filepath.Join(root, filepath.FromSlash(rel)))
		}

		assert.Equal(t, want, offered)
	})

	t.Run(
		"a file the selector drops is not read but still counts toward MaxFiles",
		func(t *testing.T) {
			t.Parallel()

			const root = "/estate"

			src := vfs.NewMemMapFS()
			writeMirrorTree(t, src, root)

			limits := vfs.MirrorLimits{MaxFiles: len(wanted), MaxBytes: bytesOf(hclOnly)}

			_, err := vfs.MirrorToMem(src, root, limits, vfs.WithSelector(selectHCL))
			require.NoError(t, err)

			limits.MaxFiles = len(hclOnly)

			_, err = vfs.MirrorToMem(src, root, limits, vfs.WithSelector(selectHCL))
			require.ErrorIs(t, err, vfs.ErrMirrorTooLarge)
		},
	)

	t.Run("a selector error fails the copy", func(t *testing.T) {
		t.Parallel()

		const root = "/estate"

		src := vfs.NewMemMapFS()
		writeMirrorTree(t, src, root)

		errSelector := errors.New("selector")

		_, err := vfs.MirrorToMem(
			src,
			root,
			generousLimits,
			vfs.WithSelector(func([]string) ([]string, error) {
				return nil, errSelector
			}),
		)
		require.ErrorIs(t, err, errSelector)
	})

	t.Run("a selector returning a path it was not handed fails the copy", func(t *testing.T) {
		t.Parallel()

		const root = "/estate"

		src := vfs.NewMemMapFS()
		writeMirrorTree(t, src, root)

		_, err := vfs.MirrorToMem(
			src,
			root,
			generousLimits,
			vfs.WithSelector(func([]string) ([]string, error) {
				return []string{filepath.Join(root, "unit", ".terraform", "providers", "p")}, nil
			}),
		)
		require.ErrorIs(t, err, vfs.ErrMirrorUnknownPath)
	})

	t.Run("filter queries fit the selector", func(t *testing.T) {
		t.Parallel()

		const root = "/estate"

		src := vfs.NewMemMapFS()
		writeMirrorTree(t, src, root)

		l := logger.CreateLogger()

		filters, err := filter.ParseFilterQueries(l, []string{"./unit/**"})
		require.NoError(t, err)

		dst, err := vfs.MirrorToMem(
			src,
			root,
			generousLimits,
			vfs.WithSelector(func(files []string) ([]string, error) {
				comps, err := filters.EvaluateOnFiles(l, files, root)
				if err != nil {
					return nil, err
				}

				return comps.Paths(), nil
			}),
		)
		require.NoError(t, err)

		assert.ElementsMatch(
			t,
			[]string{"unit/terragrunt.hcl", "unit/vars.tfvars"},
			mirroredPaths(t, dst, root),
		)
	})

	t.Run("a missing root fails the copy", func(t *testing.T) {
		t.Parallel()

		_, err := vfs.MirrorToMem(vfs.NewMemMapFS(), "/missing", generousLimits)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})
}

// TestMirrorToMemWithRacing drives the parallel walk and copy an OS source
// takes, where the callbacks run on several goroutines at once.
func TestMirrorToMemWithRacing(t *testing.T) {
	t.Parallel()

	t.Run("copies the whole tree", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		src := vfs.NewOSFS()
		writeMirrorTree(t, src, root)

		dst, err := vfs.MirrorToMem(src, root, generousLimits)
		require.NoError(t, err)

		assert.ElementsMatch(t, wanted, mirroredPaths(t, dst, root))

		for _, rel := range wanted {
			contents, err := vfs.ReadFileAsString(dst, filepath.Join(root, filepath.FromSlash(rel)))
			require.NoError(t, err)
			assert.Equal(t, mirrorTree[rel], contents, rel)
		}
	})

	t.Run("a limit tripped on one goroutine fails the copy", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		src := vfs.NewOSFS()

		// Enough directories that several workers are mid-walk when the
		// limit trips.
		for i := range 64 {
			path := filepath.Join(root, fmt.Sprintf("unit%02d", i), "d", "terragrunt.hcl")
			require.NoError(t, vfs.WriteFile(src, path, []byte("x"), 0o644))
		}

		_, err := vfs.MirrorToMem(src, root, vfs.MirrorLimits{MaxFiles: 8, MaxBytes: 1 << 20})
		require.ErrorIs(t, err, vfs.ErrMirrorTooLarge)
	})

	t.Run("the selector applies under the parallel walk", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		src := vfs.NewOSFS()
		writeMirrorTree(t, src, root)

		dst, err := vfs.MirrorToMem(src, root, generousLimits, vfs.WithSelector(selectHCL))
		require.NoError(t, err)

		assert.ElementsMatch(t, hclOnly, mirroredPaths(t, dst, root))
	})
}
