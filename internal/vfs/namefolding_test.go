package vfs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

func TestDetectNameFoldingMemMapFS(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, fsys.MkdirAll("/dir", 0o755))

	folding, err := vfs.DetectNameFolding(fsys, "/dir")
	require.NoError(t, err)
	assert.Equal(t, vfs.NameFolding{}, folding, "the in-memory filesystem keeps every spelling distinct")

	entries, err := vfs.ReadDir(fsys, "/dir")
	require.NoError(t, err)
	assert.Empty(t, entries, "the probe file must be removed")
}

// The real filesystem is the only place the answer varies, so the expectation comes from an independent lookup.
func TestDetectNameFoldingOSFS(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	name := "reference-\u00e9.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))

	want := vfs.NameFolding{
		Case:    existsOnDisk(t, filepath.Join(dir, "REFERENCE-\u00e9.TXT")),
		Unicode: existsOnDisk(t, filepath.Join(dir, norm.NFD.String(name))),
	}

	folding, err := vfs.DetectNameFolding(vfs.NewOSFS(), dir)
	require.NoError(t, err)
	assert.Equal(t, want, folding)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the reference file may remain")
}

func TestDetectNameFoldingFoldingFS(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		fold func(string) string
		name string
		want vfs.NameFolding
	}{
		{
			name: "case-insensitive",
			fold: strings.ToLower,
			want: vfs.NameFolding{Case: true},
		},
		{
			name: "normalization-insensitive",
			fold: norm.NFC.String,
			want: vfs.NameFolding{Unicode: true},
		},
		{
			name: "case- and normalization-insensitive",
			fold: func(name string) string { return strings.ToLower(norm.NFC.String(name)) },
			want: vfs.NameFolding{Case: true, Unicode: true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := vfs.NewMemMapFS()
			require.NoError(t, base.MkdirAll("/dir", 0o755))

			fsys := &statHookFS{FS: base, stat: func(name string) (os.FileInfo, error) {
				return base.Stat(tc.fold(name))
			}}

			folding, err := vfs.DetectNameFolding(fsys, "/dir")
			require.NoError(t, err)
			assert.Equal(t, tc.want, folding)

			entries, err := vfs.ReadDir(base, "/dir")
			require.NoError(t, err)
			assert.Empty(t, entries, "the probe file must be removed")
		})
	}
}

func TestDetectNameFoldingReportsFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wrap     func(base vfs.FS) vfs.FS
		name     string
		wantLeft int
	}{
		{
			name: "probe cannot be created",
			wrap: func(base vfs.FS) vfs.FS {
				return &faultFS{FS: base, faults: map[string]string{faultOpenFile: ""}}
			},
		},
		{
			name: "probe cannot be closed",
			wrap: func(base vfs.FS) vfs.FS {
				return &faultFS{FS: base, fileFaults: []string{faultClose}}
			},
		},
		{
			name: "case probe cannot be looked up",
			wrap: func(base vfs.FS) vfs.FS {
				return &faultFS{FS: base, faults: map[string]string{faultStat: ""}}
			},
		},
		{
			name: "unicode probe cannot be looked up",
			wrap: func(base vfs.FS) vfs.FS {
				return &statHookFS{FS: base, stat: func(name string) (os.FileInfo, error) {
					if !norm.NFC.IsNormalString(name) {
						return nil, &os.PathError{Op: "stat", Path: name, Err: errInjected}
					}

					return base.Stat(name)
				}}
			},
		},
		{
			name: "probe cannot be removed",
			wrap: func(base vfs.FS) vfs.FS {
				return &faultFS{FS: base, faults: map[string]string{faultRemove: ""}}
			},
			wantLeft: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := vfs.NewMemMapFS()
			require.NoError(t, base.MkdirAll("/dir", 0o755))

			folding, err := vfs.DetectNameFolding(tc.wrap(base), "/dir")
			require.ErrorIs(t, err, errInjected)
			assert.Equal(t, vfs.NameFolding{}, folding)

			entries, err := vfs.ReadDir(base, "/dir")
			require.NoError(t, err)
			assert.Len(t, entries, tc.wantLeft, "the probe file is removed unless removing it is what failed")
		})
	}
}

func existsOnDisk(t *testing.T, path string) bool {
	t.Helper()

	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}

	require.NoError(t, err)

	return true
}

// statHookFS answers Stat through stat, so a test can fold spellings together or fail chosen ones.
type statHookFS struct {
	vfs.FS
	stat func(name string) (os.FileInfo, error)
}

func (fsys *statHookFS) Stat(name string) (os.FileInfo, error) {
	return fsys.stat(name)
}
