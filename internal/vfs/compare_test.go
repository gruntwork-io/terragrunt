package vfs_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirsDiffer(t *testing.T) {
	t.Parallel()

	const blockSize = 64 * 1024

	sameSizeTail := func(last byte) []byte {
		content := make([]byte, 2*blockSize+1)
		content[len(content)-1] = last

		return content
	}

	type file struct {
		path    string
		content []byte
		perm    os.FileMode
	}

	type testCase struct {
		name   string
		from   []file
		to     []file
		differ bool
	}

	testCases := []testCase{
		{
			name: "empty_directories",
		},
		{
			name: "same_files",
			from: []file{{path: "a.txt", content: []byte("a")}, {path: "b.txt", content: []byte("b")}},
			to:   []file{{path: "b.txt", content: []byte("b")}, {path: "a.txt", content: []byte("a")}},
		},
		{
			name:   "modified_file_of_the_same_size",
			from:   []file{{path: "file.txt", content: []byte("content1")}},
			to:     []file{{path: "file.txt", content: []byte("content2")}},
			differ: true,
		},
		{
			name:   "modified_file_of_a_different_size",
			from:   []file{{path: "file.txt", content: []byte("content")}},
			to:     []file{{path: "file.txt", content: []byte("longer content")}},
			differ: true,
		},
		{
			name:   "difference_past_the_first_block",
			from:   []file{{path: "file.bin", content: sameSizeTail(1)}},
			to:     []file{{path: "file.bin", content: sameSizeTail(2)}},
			differ: true,
		},
		{
			name: "same_file_of_a_whole_number_of_blocks",
			from: []file{{path: "file.bin", content: make([]byte, 2*blockSize)}},
			to:   []file{{path: "file.bin", content: make([]byte, 2*blockSize)}},
		},
		{
			name:   "renamed_file",
			from:   []file{{path: "original.txt", content: []byte("same content")}},
			to:     []file{{path: "renamed.txt", content: []byte("same content")}},
			differ: true,
		},
		{
			name:   "file_moved_to_a_subdirectory",
			from:   []file{{path: "file.txt", content: []byte("same content")}},
			to:     []file{{path: filepath.Join("subdir", "file.txt"), content: []byte("same content")}},
			differ: true,
		},
		{
			name:   "added_file",
			from:   []file{{path: "a.txt", content: []byte("a")}},
			to:     []file{{path: "a.txt", content: []byte("a")}, {path: "b.txt"}},
			differ: true,
		},
		{
			name: "same_nested_files",
			from: []file{{path: filepath.Join("sub", "nested", "file.txt"), content: []byte("nested content")}},
			to:   []file{{path: filepath.Join("sub", "nested", "file.txt"), content: []byte("nested content")}},
		},
		{
			name:   "file_ignored_by_no_option",
			from:   []file{{path: "kept", content: []byte("one")}},
			to:     []file{{path: "kept", content: []byte("another")}},
			differ: true,
		},
		{
			name: "file_ignored_by_the_second_option",
			from: []file{{path: "skipped", content: []byte("one")}},
			to:   []file{{path: "skipped", content: []byte("another")}},
		},
		{
			name: "ignored_file_on_one_side",
			from: []file{{path: "file.txt", content: []byte("test content")}},
			to: []file{
				{path: "file.txt", content: []byte("test content")},
				{path: filepath.Join("sub", "ignored"), content: []byte("anything")},
			},
		},
		{
			name: "ignored_file_with_different_content",
			from: []file{{path: "ignored", content: []byte("one")}},
			to:   []file{{path: "ignored", content: []byte("another")}},
		},
	}

	if runtime.GOOS != "windows" {
		testCases = append(testCases, testCase{
			name:   "changed_permissions",
			from:   []file{{path: "run.sh", content: []byte("#!/usr/bin/env bash"), perm: 0644}},
			to:     []file{{path: "run.sh", content: []byte("#!/usr/bin/env bash"), perm: 0755}},
			differ: true,
		})
	}

	ignore := []vfs.DirsDifferOption{
		vfs.WithIgnore(func(d fs.DirEntry) bool { return d.Name() == "ignored" }),
		vfs.WithIgnore(func(d fs.DirEntry) bool { return d.Name() == "skipped" }),
	}

	writeFiles := func(t *testing.T, fsys vfs.FS, dir string, files []file) {
		t.Helper()

		require.NoError(t, fsys.MkdirAll(dir, 0755))

		for _, f := range files {
			path := filepath.Join(dir, f.path)
			require.NoError(t, fsys.MkdirAll(filepath.Dir(path), 0755))

			perm := f.perm
			if perm == 0 {
				perm = 0644
			}

			require.NoError(t, vfs.WriteFile(fsys, path, f.content, perm))
		}
	}

	backends := []struct {
		new  func(t *testing.T) (fsys vfs.FS, root string)
		name string
	}{
		{
			name: "os",
			new: func(t *testing.T) (vfs.FS, string) {
				t.Helper()

				return vfs.NewOSFS(), t.TempDir()
			},
		},
		{
			name: "mem",
			new: func(t *testing.T) (vfs.FS, string) {
				t.Helper()

				return vfs.NewMemMapFS(), string(filepath.Separator)
			},
		},
	}

	for _, backend := range backends {
		for _, tc := range testCases {
			t.Run(backend.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				fsys, root := backend.new(t)
				fromDir := filepath.Join(root, "from")
				toDir := filepath.Join(root, "to")

				writeFiles(t, fsys, fromDir, tc.from)
				writeFiles(t, fsys, toDir, tc.to)

				differ, err := vfs.DirsDiffer(fsys, fromDir, toDir, ignore...)
				require.NoError(t, err)
				assert.Equal(t, tc.differ, differ)

				differ, err = vfs.DirsDiffer(fsys, toDir, fromDir, ignore...)
				require.NoError(t, err)
				assert.Equal(t, tc.differ, differ)
			})
		}

		t.Run(backend.name+"/missing_directory", func(t *testing.T) {
			t.Parallel()

			fsys, root := backend.new(t)
			fromDir := filepath.Join(root, "from")

			require.NoError(t, fsys.MkdirAll(fromDir, 0755))

			_, err := vfs.DirsDiffer(fsys, fromDir, filepath.Join(root, "missing"))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}
