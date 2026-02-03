package vfs_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOSFS(t *testing.T) {
	t.Parallel()

	fs := vfs.NewOSFS()

	assert.NotNil(t, fs)
	_, ok := fs.(*afero.OsFs)
	assert.True(t, ok, "expected *afero.OsFs type")
}

func TestNewMemMapFS(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	assert.NotNil(t, fs)
	_, ok := fs.(*afero.MemMapFs)
	assert.True(t, ok, "expected *afero.MemMapFs type")
}

func TestFileExists(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		setup    func(fs vfs.FS)
		path     string
		expected bool
	}{
		{
			name: "file exists",
			setup: func(fs vfs.FS) {
				require.NoError(t, afero.WriteFile(fs, "/test.txt", []byte("content"), 0644))
			},
			path:     "/test.txt",
			expected: true,
		},
		{
			name:     "file does not exist",
			setup:    func(fs vfs.FS) {},
			path:     "/nonexistent.txt",
			expected: false,
		},
		{
			name: "directory exists",
			setup: func(fs vfs.FS) {
				require.NoError(t, fs.MkdirAll("/testdir", 0755))
			},
			path:     "/testdir",
			expected: true,
		},
		{
			name:     "parent does not exist",
			setup:    func(fs vfs.FS) {},
			path:     "/nonexistent/file.txt",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()
			tc.setup(fs)

			exists, err := vfs.FileExists(fs, tc.path)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, exists)
		})
	}
}

func TestWriteFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		filename string
		data     []byte
		perm     os.FileMode
	}{
		{
			name:     "write simple file",
			filename: "/test.txt",
			data:     []byte("hello world"),
			perm:     0644,
		},
		{
			name:     "write with restricted permissions",
			filename: "/restricted.txt",
			data:     []byte("secret"),
			perm:     0600,
		},
		{
			name:     "write to nested directory",
			filename: "/nested/path/file.txt",
			data:     []byte("nested content"),
			perm:     0644,
		},
		{
			name:     "write empty file",
			filename: "/empty.txt",
			data:     []byte{},
			perm:     0644,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()

			err := vfs.WriteFile(fs, tc.filename, tc.data, tc.perm)

			require.NoError(t, err)

			exists, err := vfs.FileExists(fs, tc.filename)
			require.NoError(t, err)
			assert.True(t, exists)
		})
	}
}

func TestReadFile(t *testing.T) {
	t.Parallel()

	t.Run("read existing file", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		expected := []byte("test content")
		require.NoError(t, vfs.WriteFile(fs, "/test.txt", expected, 0644))

		data, err := vfs.ReadFile(fs, "/test.txt")

		require.NoError(t, err)
		assert.Equal(t, expected, data)
	})

	t.Run("read non-existent file", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()

		_, err := vfs.ReadFile(fs, "/nonexistent.txt")

		require.Error(t, err)
	})
}

func TestSymlink(t *testing.T) {
	t.Parallel()

	t.Run("create valid symlink", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewOSFS()
		tempDir := t.TempDir()
		targetPath := filepath.Join(tempDir, "target.txt")
		linkPath := filepath.Join(tempDir, "link.txt")

		require.NoError(t, vfs.WriteFile(fs, targetPath, []byte("target content"), 0644))

		err := vfs.Symlink(fs, targetPath, linkPath)

		require.NoError(t, err)

		data, err := vfs.ReadFile(fs, linkPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("target content"), data)
	})

	t.Run("symlink to non-existent target", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewOSFS()
		tempDir := t.TempDir()
		linkPath := filepath.Join(tempDir, "dangling_link.txt")

		err := vfs.Symlink(fs, "/nonexistent/target", linkPath)

		require.NoError(t, err)
	})

	t.Run("filesystem without symlink support returns LinkError", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewReadOnlyFs(vfs.NewMemMapFS())

		err := vfs.Symlink(fs, "target", "link")

		require.Error(t, err)

		var linkErr *os.LinkError
		assert.ErrorAs(t, err, &linkErr)
	})
}

func TestUnzip(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("extract single file", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchive(t, map[string][]byte{
			"file.txt": []byte("file content"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.NoError(t, err)

		data, err := vfs.ReadFile(fs, "/dst/file.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("file content"), data)
	})

	t.Run("extract archive with directories", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchiveWithDirs(t, map[string][]byte{
			"dir/":         nil,
			"dir/file.txt": []byte("nested file"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.NoError(t, err)

		exists, err := vfs.FileExists(fs, "/dst/dir")
		require.NoError(t, err)
		assert.True(t, exists)

		data, err := vfs.ReadFile(fs, "/dst/dir/file.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("nested file"), data)
	})

	t.Run("extract archive with nested structure", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchive(t, map[string][]byte{
			"a/b/c/deep.txt": []byte("deep content"),
			"root.txt":       []byte("root content"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.NoError(t, err)

		data, err := vfs.ReadFile(fs, "/dst/a/b/c/deep.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("deep content"), data)

		data, err = vfs.ReadFile(fs, "/dst/root.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("root content"), data)
	})

	t.Run("extract archive with multiple files", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchive(t, map[string][]byte{
			"file1.txt": []byte("content1"),
			"file2.txt": []byte("content2"),
			"file3.txt": []byte("content3"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.NoError(t, err)

		for i := 1; i <= 3; i++ {
			data, err := vfs.ReadFile(fs, "/dst/file"+string(rune('0'+i))+".txt")
			require.NoError(t, err)
			assert.Equal(t, []byte("content"+string(rune('0'+i))), data)
		}
	})

	t.Run("zipslip prevention - path with dotdot", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchiveUnsafe(t, map[string][]byte{
			"../escaped.txt": []byte("malicious"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "illegal file path")
	})

	t.Run("zipslip prevention - nested dotdot", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchiveUnsafe(t, map[string][]byte{
			"foo/../../escaped.txt": []byte("malicious"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "illegal file path")
	})

	t.Run("permissions preserved with umask 0", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewOSFS()
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "archive.zip")
		dstPath := filepath.Join(tempDir, "dst")

		zipData := createZipArchiveWithMode(t, "executable.sh", []byte("#!/bin/bash"), 0755)
		require.NoError(t, vfs.WriteFile(fs, zipPath, zipData, 0644))

		err := vfs.Unzip(l, fs, dstPath, zipPath, 0)

		require.NoError(t, err)

		info, err := fs.Stat(filepath.Join(dstPath, "executable.sh"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
	})

	t.Run("permissions with umask applied", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewOSFS()
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "archive.zip")
		dstPath := filepath.Join(tempDir, "dst")

		zipData := createZipArchiveWithMode(t, "file.txt", []byte("content"), 0666)
		require.NoError(t, vfs.WriteFile(fs, zipPath, zipData, 0644))

		err := vfs.Unzip(l, fs, dstPath, zipPath, 0022)

		require.NoError(t, err)

		info, err := fs.Stat(filepath.Join(dstPath, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
	})

	t.Run("non-existent source file", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()

		err := vfs.Unzip(l, fs, "/dst", "/nonexistent.zip", 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open zip archive")
	})

	t.Run("invalid archive", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fs, "/invalid.zip", []byte("not a zip file"), 0644))

		err := vfs.Unzip(l, fs, "/dst", "/invalid.zip", 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read zip archive")
	})

	t.Run("extract to existing directory", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		require.NoError(t, fs.MkdirAll("/dst", 0755))
		zipData := createZipArchive(t, map[string][]byte{
			"new.txt": []byte("new content"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.NoError(t, err)

		data, err := vfs.ReadFile(fs, "/dst/new.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("new content"), data)
	})
}

func TestUnzipWithSymlinks(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	fs := vfs.NewOSFS()
	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "archive.zip")
	dstPath := filepath.Join(tempDir, "dst")

	zipData := createZipArchiveWithSymlink(t, "target.txt", []byte("target content"), "link.txt", "target.txt")
	require.NoError(t, vfs.WriteFile(fs, zipPath, zipData, 0644))

	err := vfs.Unzip(l, fs, dstPath, zipPath, 0)

	require.NoError(t, err)

	targetData, err := vfs.ReadFile(fs, filepath.Join(dstPath, "target.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("target content"), targetData)

	linkData, err := vfs.ReadFile(fs, filepath.Join(dstPath, "link.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("target content"), linkData)
}

// createZipArchive creates a zip archive in memory with the given files.
func createZipArchive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)

	for name, content := range files {
		f, err := w.Create(name)
		require.NoError(t, err)

		_, err = f.Write(content)
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())

	return buf.Bytes()
}

// createZipArchiveWithDirs creates a zip archive that includes directory entries.
func createZipArchiveWithDirs(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)

	for name, content := range files {
		if content == nil {
			_, err := w.Create(name)
			require.NoError(t, err)

			continue
		}

		f, err := w.Create(name)
		require.NoError(t, err)

		_, err = f.Write(content)
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())

	return buf.Bytes()
}

// createZipArchiveUnsafe creates a zip archive with potentially malicious paths (for testing ZipSlip).
func createZipArchiveUnsafe(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)

	for name, content := range files {
		header := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}

		f, err := w.CreateHeader(header)
		require.NoError(t, err)

		_, err = f.Write(content)
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())

	return buf.Bytes()
}

// createZipArchiveWithMode creates a zip archive with a single file with specific permissions.
func createZipArchiveWithMode(t *testing.T, name string, content []byte, mode os.FileMode) []byte {
	t.Helper()

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)

	header := &zip.FileHeader{
		Name:   name,
		Method: zip.Deflate,
	}
	header.SetMode(mode)

	f, err := w.CreateHeader(header)
	require.NoError(t, err)

	_, err = f.Write(content)
	require.NoError(t, err)

	require.NoError(t, w.Close())

	return buf.Bytes()
}

// createZipArchiveWithSymlink creates a zip archive with a regular file and a symlink to it.
func createZipArchiveWithSymlink(t *testing.T, targetName string, targetContent []byte, linkName, linkTarget string) []byte {
	t.Helper()

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)

	targetHeader := &zip.FileHeader{
		Name:   targetName,
		Method: zip.Deflate,
	}
	targetHeader.SetMode(0644)

	f, err := w.CreateHeader(targetHeader)
	require.NoError(t, err)

	_, err = f.Write(targetContent)
	require.NoError(t, err)

	linkHeader := &zip.FileHeader{
		Name:   linkName,
		Method: zip.Deflate,
	}
	linkHeader.SetMode(os.ModeSymlink | 0777)

	linkFile, err := w.CreateHeader(linkHeader)
	require.NoError(t, err)

	_, err = linkFile.Write([]byte(linkTarget))
	require.NoError(t, err)

	require.NoError(t, w.Close())

	return buf.Bytes()
}
