package vfs_test

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
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

	_, ok := fs.(afero.Symlinker)
	assert.True(t, ok, "expected OSFS to implement afero.Symlinker")

	_, ok = fs.(vfs.HardLinker)
	assert.True(t, ok, "expected OSFS to implement vfs.HardLinker")
}

func TestNewMemMapFS(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()

	assert.NotNil(t, fs)

	_, ok := fs.(afero.Symlinker)
	assert.True(t, ok, "expected MemMapFS to implement afero.Symlinker")

	_, ok = fs.(vfs.HardLinker)
	assert.True(t, ok, "expected MemMapFS to implement vfs.HardLinker")
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

func TestEvalSymlinks(t *testing.T) {
	t.Parallel()

	t.Run("returns regular path unchanged", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		require.NoError(t, fs.MkdirAll("/root/real/sub", 0o755))

		resolved, err := vfs.EvalSymlinks(fs, "/root/real/sub")

		require.NoError(t, err)
		assert.Equal(t, "/root/real/sub", resolved)
	})

	t.Run("resolves parent symlink", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		require.NoError(t, fs.MkdirAll("/root/real/sub", 0o755))
		require.NoError(t, vfs.Symlink(fs, "/root/real", "/root/link"))

		resolved, err := vfs.EvalSymlinks(fs, "/root/link/sub")

		require.NoError(t, err)
		assert.Equal(t, "/root/real/sub", resolved)
	})

	t.Run("missing path returns error", func(t *testing.T) {
		t.Parallel()

		_, err := vfs.EvalSymlinks(vfs.NewMemMapFS(), "/root/missing")

		require.Error(t, err)
	})
}

func TestParentPathHasSymlink(t *testing.T) {
	t.Parallel()

	t.Run("parent path contains symlink", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		require.NoError(t, fs.MkdirAll("/root/real", 0o755))
		require.NoError(t, vfs.Symlink(fs, "/root/real", "/root/link"))

		hasSymlink, err := vfs.ParentPathHasSymlink(fs, "/root", "link/file.txt")

		require.NoError(t, err)
		assert.True(t, hasSymlink)
	})

	t.Run("leaf symlink is not considered parent symlink", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		require.NoError(t, fs.MkdirAll("/root/real", 0o755))
		require.NoError(t, vfs.Symlink(fs, "/root/real", "/root/link"))

		hasSymlink, err := vfs.ParentPathHasSymlink(fs, "/root", "link")

		require.NoError(t, err)
		assert.False(t, hasSymlink)
	})

	t.Run("regular parents", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		require.NoError(t, fs.MkdirAll("/root/real", 0o755))

		hasSymlink, err := vfs.ParentPathHasSymlink(fs, "/root", "real/file.txt")

		require.NoError(t, err)
		assert.False(t, hasSymlink)
	})

	t.Run("missing parent is not treated as symlink", func(t *testing.T) {
		t.Parallel()

		hasSymlink, err := vfs.ParentPathHasSymlink(vfs.NewMemMapFS(), "/root", "missing/file.txt")

		require.NoError(t, err)
		assert.False(t, hasSymlink)
	})

	t.Run("current directory is unsafe", func(t *testing.T) {
		t.Parallel()

		hasSymlink, err := vfs.ParentPathHasSymlink(vfs.NewMemMapFS(), "/root", ".")

		require.NoError(t, err)
		assert.True(t, hasSymlink)
	})

	t.Run("absolute relative path is unsafe", func(t *testing.T) {
		t.Parallel()

		hasSymlink, err := vfs.ParentPathHasSymlink(vfs.NewMemMapFS(), "/root", "/root/file.txt")

		require.NoError(t, err)
		assert.True(t, hasSymlink)
	})

	t.Run("invalid relative path is unsafe", func(t *testing.T) {
		t.Parallel()

		hasSymlink, err := vfs.ParentPathHasSymlink(vfs.NewMemMapFS(), "/root", "../escape.txt")

		require.NoError(t, err)
		assert.True(t, hasSymlink)
	})

	t.Run("deep nested parent symlink", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		require.NoError(t, fs.MkdirAll("/root/a/b/real", 0o755))
		require.NoError(t, vfs.Symlink(fs, "/root/a/b/real", "/root/a/b/link"))

		hasSymlink, err := vfs.ParentPathHasSymlink(fs, "/root", "a/b/link/file.txt")

		require.NoError(t, err)
		assert.True(t, hasSymlink)
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

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

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

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

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

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

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

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

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

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

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

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

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

		err := vfs.NewZipDecompressor().Unzip(l, fs, dstPath, zipPath, 0)

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

		err := vfs.NewZipDecompressor().Unzip(l, fs, dstPath, zipPath, 0022)

		require.NoError(t, err)

		info, err := fs.Stat(filepath.Join(dstPath, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
	})

	t.Run("non-existent source file", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/nonexistent.zip", 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open zip archive")
	})

	t.Run("invalid archive", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fs, "/invalid.zip", []byte("not a zip file"), 0644))

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/invalid.zip", 0)

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

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

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

	err := vfs.NewZipDecompressor().Unzip(l, fs, dstPath, zipPath, 0)

	require.NoError(t, err)

	targetData, err := vfs.ReadFile(fs, filepath.Join(dstPath, "target.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("target content"), targetData)

	linkData, err := vfs.ReadFile(fs, filepath.Join(dstPath, "link.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("target content"), linkData)
}

func TestContainsDotDot(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("allows file with double dots in name", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchive(t, map[string][]byte{
			"file..txt": []byte("content with dots"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.NoError(t, err)

		data, err := vfs.ReadFile(fs, "/dst/file..txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("content with dots"), data)
	})

	t.Run("allows file with multiple dots", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchive(t, map[string][]byte{
			"my..file..name.txt": []byte("multiple dots"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.NoError(t, err)

		data, err := vfs.ReadFile(fs, "/dst/my..file..name.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("multiple dots"), data)
	})

	t.Run("blocks path with dotdot component", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchiveUnsafe(t, map[string][]byte{
			"../evil.txt": []byte("malicious"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "illegal file path")
	})

	t.Run("blocks nested dotdot path", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchiveUnsafe(t, map[string][]byte{
			"subdir/../../../evil.txt": []byte("malicious"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "illegal file path")
	})
}

func TestUnzipFilesLimit(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("allows extraction within file limit", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchive(t, map[string][]byte{
			"file1.txt": []byte("content1"),
			"file2.txt": []byte("content2"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.NewZipDecompressor(vfs.WithFilesLimit(5)).Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.NoError(t, err)

		exists, err := vfs.FileExists(fs, "/dst/file1.txt")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("rejects extraction exceeding file limit", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchive(t, map[string][]byte{
			"file1.txt": []byte("content1"),
			"file2.txt": []byte("content2"),
			"file3.txt": []byte("content3"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.NewZipDecompressor(vfs.WithFilesLimit(2)).Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})

	t.Run("no limit when FilesLimit is zero", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchive(t, map[string][]byte{
			"file1.txt": []byte("content1"),
			"file2.txt": []byte("content2"),
			"file3.txt": []byte("content3"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.NoError(t, err)
	})
}

func TestUnzipFileSizeLimit(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("allows extraction within size limit", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchive(t, map[string][]byte{
			"small.txt": []byte("small content"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.NewZipDecompressor(vfs.WithFileSizeLimit(1000)).Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.NoError(t, err)

		data, err := vfs.ReadFile(fs, "/dst/small.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("small content"), data)
	})

	t.Run("rejects extraction exceeding size limit", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		// Create content that exceeds 10 bytes
		zipData := createZipArchive(t, map[string][]byte{
			"large.txt": []byte("this content is definitely more than 10 bytes"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.NewZipDecompressor(vfs.WithFileSizeLimit(10)).Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})

	t.Run("cumulative size limit across files", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		// Each file is 10 bytes, total 30 bytes
		zipData := createZipArchive(t, map[string][]byte{
			"file1.txt": []byte("0123456789"),
			"file2.txt": []byte("0123456789"),
			"file3.txt": []byte("0123456789"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.NewZipDecompressor(vfs.WithFileSizeLimit(25)).Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})

	t.Run("no limit when FileSizeLimit is zero", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		zipData := createZipArchive(t, map[string][]byte{
			"file.txt": []byte("content that would exceed any small limit"),
		})
		require.NoError(t, vfs.WriteFile(fs, "/archive.zip", zipData, 0644))

		err := vfs.NewZipDecompressor().Unzip(l, fs, "/dst", "/archive.zip", 0)

		require.NoError(t, err)
	})
}

func TestUnzipSymlinkEscape(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("allows symlink to file within destination", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewOSFS()
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "archive.zip")
		dstPath := filepath.Join(tempDir, "dst")

		zipData := createZipArchiveWithSymlink(t, "target.txt", []byte("target content"), "link.txt", "target.txt")
		require.NoError(t, vfs.WriteFile(fs, zipPath, zipData, 0644))

		err := vfs.NewZipDecompressor().Unzip(l, fs, dstPath, zipPath, 0)

		require.NoError(t, err)

		linkData, err := vfs.ReadFile(fs, filepath.Join(dstPath, "link.txt"))
		require.NoError(t, err)
		assert.Equal(t, []byte("target content"), linkData)
	})

	t.Run("rejects symlink escaping destination with absolute path", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewOSFS()
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "archive.zip")
		dstPath := filepath.Join(tempDir, "dst")

		// Create symlink pointing to absolute path outside destination
		zipData := createZipArchiveWithSymlink(t, "target.txt", []byte("target content"), "evil_link.txt", "/etc/passwd")
		require.NoError(t, vfs.WriteFile(fs, zipPath, zipData, 0644))

		err := vfs.NewZipDecompressor().Unzip(l, fs, dstPath, zipPath, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "symlink target escapes destination")
	})

	t.Run("rejects symlink escaping destination with relative path", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewOSFS()
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "archive.zip")
		dstPath := filepath.Join(tempDir, "dst")

		// Create symlink pointing outside destination with ..
		zipData := createZipArchiveWithSymlink(
			t, "target.txt", []byte("target content"), "evil_link.txt", "../../../etc/passwd",
		)
		require.NoError(t, vfs.WriteFile(fs, zipPath, zipData, 0644))

		err := vfs.NewZipDecompressor().Unzip(l, fs, dstPath, zipPath, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "symlink target escapes destination")
	})

	t.Run("allows symlink within nested directory", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewOSFS()
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "archive.zip")
		dstPath := filepath.Join(tempDir, "dst")

		// Create symlink in subdirectory pointing to file in same directory
		zipData := createZipArchiveWithNestedSymlink(t)
		require.NoError(t, vfs.WriteFile(fs, zipPath, zipData, 0644))

		err := vfs.NewZipDecompressor().Unzip(l, fs, dstPath, zipPath, 0)

		require.NoError(t, err)
	})
}

func TestWalkDir(t *testing.T) {
	t.Parallel()

	t.Run("walks files in lexical order", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(memFs, "/root/b.txt", []byte("b"), 0644))
		require.NoError(t, vfs.WriteFile(memFs, "/root/a.txt", []byte("a"), 0644))
		require.NoError(t, vfs.WriteFile(memFs, "/root/c.txt", []byte("c"), 0644))

		var names []string

		err := vfs.WalkDir(memFs, "/root", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			names = append(names, d.Name())

			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, []string{"root", "a.txt", "b.txt", "c.txt"}, names)
	})

	t.Run("walks nested directories", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(memFs, "/root/dir/nested.txt", []byte("n"), 0644))
		require.NoError(t, vfs.WriteFile(memFs, "/root/top.txt", []byte("t"), 0644))

		var paths []string

		err := vfs.WalkDir(memFs, "/root", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			paths = append(paths, path)

			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, []string{"/root", "/root/dir", "/root/dir/nested.txt", "/root/top.txt"}, paths)
	})

	t.Run("empty directory", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, memFs.MkdirAll("/empty", 0755))

		var paths []string

		err := vfs.WalkDir(memFs, "/empty", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			paths = append(paths, path)

			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, []string{"/empty"}, paths)
	})

	t.Run("SkipDir skips directory", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(memFs, "/root/skip/hidden.txt", []byte("h"), 0644))
		require.NoError(t, vfs.WriteFile(memFs, "/root/keep/visible.txt", []byte("v"), 0644))

		var paths []string

		err := vfs.WalkDir(memFs, "/root", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() && d.Name() == "skip" {
				return filepath.SkipDir
			}

			paths = append(paths, path)

			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, []string{"/root", "/root/keep", "/root/keep/visible.txt"}, paths)
	})

	t.Run("nonexistent root returns error to callback", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()

		var callbackErr error

		err := vfs.WalkDir(memFs, "/nonexistent", func(path string, d fs.DirEntry, err error) error {
			callbackErr = err
			return err
		})

		require.Error(t, err)
		require.Error(t, callbackErr)
	})

	t.Run("DirEntry reports correct types", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, memFs.MkdirAll("/root/subdir", 0755))
		require.NoError(t, vfs.WriteFile(memFs, "/root/file.txt", []byte("f"), 0644))

		var dirs, files int

		err := vfs.WalkDir(memFs, "/root", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				dirs++
			} else {
				files++
			}

			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, 2, dirs)  // /root, /root/subdir
		assert.Equal(t, 1, files) // /root/file.txt
	})
}

func TestWalkDir_OSFS(t *testing.T) {
	t.Parallel()

	osFs := vfs.NewOSFS()
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("b"), 0644))

	var paths []string

	err := vfs.WalkDir(osFs, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(root, path)
		require.NoError(t, relErr)

		paths = append(paths, rel)

		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{".", "a.txt", "sub", filepath.Join("sub", "b.txt")}, paths)
}

func TestReadDirEntries(t *testing.T) {
	t.Parallel()

	t.Run("returns sorted entries from MemMapFS", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fsys, "/dir/charlie.txt", []byte("c"), 0644))
		require.NoError(t, vfs.WriteFile(fsys, "/dir/alpha.txt", []byte("a"), 0644))
		require.NoError(t, vfs.WriteFile(fsys, "/dir/bravo.txt", []byte("b"), 0644))
		require.NoError(t, vfs.WriteFile(fsys, "/dir/sub/nested.txt", []byte("n"), 0644))

		entries, err := vfs.ReadDirEntries(fsys, "/dir")
		require.NoError(t, err)

		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}

		assert.Equal(t, []string{"alpha.txt", "bravo.txt", "charlie.txt", "sub"}, names)

		var subEntry fs.DirEntry

		for _, e := range entries {
			if e.Name() == "sub" {
				subEntry = e
				break
			}
		}

		require.NotNil(t, subEntry)
		assert.True(t, subEntry.IsDir())
	})

	t.Run("returns sorted entries from OSFS", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "z.txt"), []byte("z"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644))
		require.NoError(t, os.Mkdir(filepath.Join(dir, "m"), 0755))

		entries, err := vfs.ReadDirEntries(vfs.NewOSFS(), dir)
		require.NoError(t, err)

		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}

		assert.Equal(t, []string{"a.txt", "m", "z.txt"}, names)
	})

	t.Run("returns empty slice for empty directory", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, fsys.Mkdir("/empty", 0755))

		entries, err := vfs.ReadDirEntries(fsys, "/empty")
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("returns error for missing directory", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()

		_, err := vfs.ReadDirEntries(fsys, "/does-not-exist")
		require.Error(t, err)
	})
}

func TestLock(t *testing.T) {
	t.Parallel()

	t.Run("memMapFS lock and unlock", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()

		lock, err := vfs.Lock(memFs, "test.lock")
		require.NoError(t, err)
		require.NotNil(t, lock)

		// TryLock should fail while held
		_, acquired, err := vfs.TryLock(memFs, "test.lock")
		require.NoError(t, err)
		assert.False(t, acquired)

		require.NoError(t, lock.Unlock())

		// TryLock should succeed after unlock
		lock2, acquired, err := vfs.TryLock(memFs, "test.lock")
		require.NoError(t, err)
		assert.True(t, acquired)
		require.NoError(t, lock2.Unlock())
	})

	t.Run("osFS lock and unlock", func(t *testing.T) {
		t.Parallel()

		osFs := vfs.NewOSFS()
		lockPath := filepath.Join(t.TempDir(), "test.lock")

		lock, err := vfs.Lock(osFs, lockPath)
		require.NoError(t, err)
		require.NotNil(t, lock)
		require.NoError(t, lock.Unlock())
	})

	t.Run("unsupported filesystem returns error", func(t *testing.T) {
		t.Parallel()

		readOnlyFs := afero.NewReadOnlyFs(vfs.NewMemMapFS())

		_, err := vfs.Lock(readOnlyFs, "test.lock")
		require.ErrorIs(t, err, vfs.ErrNoLock)

		_, _, err = vfs.TryLock(readOnlyFs, "test.lock")
		require.ErrorIs(t, err, vfs.ErrNoLock)
	})
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
func createZipArchiveWithSymlink(
	t *testing.T, targetName string, targetContent []byte, linkName, linkTarget string,
) []byte {
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

// createZipArchiveWithNestedSymlink creates a zip with a symlink in a subdirectory.
func createZipArchiveWithNestedSymlink(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)

	// Create target file in subdir
	targetHeader := &zip.FileHeader{
		Name:   "subdir/target.txt",
		Method: zip.Deflate,
	}
	targetHeader.SetMode(0644)

	f, err := w.CreateHeader(targetHeader)
	require.NoError(t, err)

	_, err = f.Write([]byte("target content"))
	require.NoError(t, err)

	// Create symlink in same subdir pointing to target
	linkHeader := &zip.FileHeader{
		Name:   "subdir/link.txt",
		Method: zip.Deflate,
	}
	linkHeader.SetMode(os.ModeSymlink | 0777)

	linkFile, err := w.CreateHeader(linkHeader)
	require.NoError(t, err)

	_, err = linkFile.Write([]byte("target.txt"))
	require.NoError(t, err)

	require.NoError(t, w.Close())

	return buf.Bytes()
}

func TestWalkDirParallel(t *testing.T) {
	t.Parallel()

	t.Run("degrades to sequential WalkDir on MemMapFS", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(memFs, "/root/a.txt", []byte("a"), 0644))
		require.NoError(t, vfs.WriteFile(memFs, "/root/b.txt", []byte("b"), 0644))
		require.NoError(t, vfs.WriteFile(memFs, "/root/c.txt", []byte("c"), 0644))

		var names []string

		err := vfs.WalkDirParallel(memFs, "/root", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			names = append(names, d.Name())

			return nil
		})

		require.NoError(t, err)
		// Non-OS FS keeps the deterministic lexical ordering from WalkDir.
		assert.Equal(t, []string{"root", "a.txt", "b.txt", "c.txt"}, names)
	})

	t.Run("walks every entry on OSFS", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		osFs := vfs.NewOSFS()

		require.NoError(t, osFs.MkdirAll(filepath.Join(root, "dir"), 0o755))
		require.NoError(t, vfs.WriteFile(osFs, filepath.Join(root, "a.txt"), []byte("a"), 0o644))
		require.NoError(t, vfs.WriteFile(osFs, filepath.Join(root, "dir", "nested.txt"), []byte("n"), 0o644))

		var mu sync.Mutex

		seen := map[string]struct{}{}

		err := vfs.WalkDirParallel(osFs, root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			mu.Lock()
			seen[path] = struct{}{}
			mu.Unlock()

			return nil
		})

		require.NoError(t, err)
		// Parallel walk gives no ordering guarantee, but it must visit every
		// entry exactly once.
		assert.Equal(t, map[string]struct{}{
			root:                                     {},
			filepath.Join(root, "a.txt"):             {},
			filepath.Join(root, "dir"):               {},
			filepath.Join(root, "dir", "nested.txt"): {},
		}, seen)
	})

	t.Run("SkipDir prunes a subtree on OSFS", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		osFs := vfs.NewOSFS()

		require.NoError(t, osFs.MkdirAll(filepath.Join(root, "keep"), 0o755))
		require.NoError(t, osFs.MkdirAll(filepath.Join(root, "skip"), 0o755))
		require.NoError(t, vfs.WriteFile(osFs, filepath.Join(root, "keep", "a.txt"), []byte("a"), 0o644))
		require.NoError(t, vfs.WriteFile(osFs, filepath.Join(root, "skip", "b.txt"), []byte("b"), 0o644))

		var mu sync.Mutex

		seen := map[string]struct{}{}

		err := vfs.WalkDirParallel(osFs, root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() && d.Name() == "skip" {
				return filepath.SkipDir
			}

			mu.Lock()
			seen[path] = struct{}{}
			mu.Unlock()

			return nil
		})

		require.NoError(t, err)
		assert.Contains(t, seen, filepath.Join(root, "keep", "a.txt"))
		assert.NotContains(t, seen, filepath.Join(root, "skip", "b.txt"))
	})
}
