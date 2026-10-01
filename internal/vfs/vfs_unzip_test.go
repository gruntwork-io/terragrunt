package vfs_test

import (
	"archive/zip"
	"bytes"
	"hash/crc32"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unsupportedZipMethod is a compression method archive/zip cannot decompress.
const unsupportedZipMethod = 99

// corruptDeflate is not a valid deflate stream because its first block uses the reserved type.
var corruptDeflate = []byte{0xff, 0xff, 0xff, 0xff}

func TestUnzipEntryFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr   error
		faults    map[string]string
		name      string
		wantMsg   string
		wantWarn  string
		wantGone  string
		fileFault []string
		entry     rawZipEntry
	}{
		{
			name:    "file with an unsupported compression method",
			entry:   rawZipEntry{name: "file.txt", method: unsupportedZipMethod, data: []byte("x"), mode: 0o644},
			wantErr: zip.ErrAlgorithm,
			wantMsg: "failed to open file",
		},
		{
			name:    "symlink with an unsupported compression method",
			entry:   rawZipEntry{name: "link", method: unsupportedZipMethod, data: []byte("x"), mode: os.ModeSymlink | 0o777},
			wantErr: zip.ErrAlgorithm,
			wantMsg: "failed to open file",
		},
		{
			name:     "file whose data does not decompress is removed",
			entry:    rawZipEntry{name: "file.txt", method: zip.Deflate, data: corruptDeflate, mode: 0o644},
			wantMsg:  "failed to copy file",
			wantWarn: "Error closing file",
			wantGone: "/dst/file.txt",
		},
		{
			name: "cleanup failures after a failed copy are logged",
			entry: rawZipEntry{
				name: "file.txt", method: zip.Deflate, data: corruptDeflate, mode: 0o644,
			},
			faults:    map[string]string{faultRemove: "/dst/file.txt"},
			fileFault: []string{faultClose},
			wantMsg:   "failed to copy file",
			wantWarn:  "Error removing partial file",
		},
		{
			name:     "symlink whose data does not decompress",
			entry:    rawZipEntry{name: "link", method: zip.Deflate, data: corruptDeflate, mode: os.ModeSymlink | 0o777},
			wantMsg:  "failed to read file",
			wantWarn: "Error closing file",
		},
		{
			name:    "file whose directory cannot be created",
			entry:   rawZipEntry{name: "sub/file.txt", method: zip.Store, data: []byte("x"), mode: 0o644},
			faults:  map[string]string{faultMkdirAll: "/dst/sub"},
			wantErr: errInjected,
			wantMsg: "failed to create directory",
		},
		{
			name:    "directory entry that cannot be created",
			entry:   rawZipEntry{name: "sub/", method: zip.Store, mode: os.ModeDir | 0o755},
			faults:  map[string]string{faultMkdirAll: "/dst/sub"},
			wantErr: errInjected,
			wantMsg: "failed to create directory",
		},
		{
			name:    "symlink whose directory cannot be created",
			entry:   rawZipEntry{name: "sub/link", method: zip.Store, data: []byte("target"), mode: os.ModeSymlink | 0o777},
			faults:  map[string]string{faultMkdirAll: "/dst/sub"},
			wantErr: errInjected,
			wantMsg: "failed to create directory",
		},
		{
			name:    "file that cannot be created",
			entry:   rawZipEntry{name: "file.txt", method: zip.Store, data: []byte("x"), mode: 0o644},
			faults:  map[string]string{faultOpenFile: "/dst/file.txt"},
			wantErr: errInjected,
			wantMsg: "failed to create file",
		},
		{
			name:    "entry naming the destination itself",
			entry:   rawZipEntry{name: ".", method: zip.Store, data: []byte("x"), mode: 0o644},
			wantMsg: "illegal destination path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(base, "/archive.zip", createRawZipArchive(t, tc.entry), 0o644))

			fsys := &faultFS{FS: base, faults: tc.faults, fileFaults: tc.fileFault}

			var out bytes.Buffer

			l := logger.CreateLogger().WithOptions(log.WithOutput(&out))

			err := vfs.NewZipDecompressor().Unzip(l, fsys, "/dst", "/archive.zip", 0)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}

			if tc.wantWarn != "" {
				assert.Contains(t, out.String(), tc.wantWarn)
			}

			if tc.wantGone != "" {
				assert.False(t, vfs.Exists(base, tc.wantGone), "a partial file must not be left behind")
			}
		})
	}
}

func TestUnzipArchiveFailures(t *testing.T) {
	t.Parallel()

	archive := createZipArchive(t, map[string][]byte{"file.txt": []byte("content")})

	t.Run("archive that cannot be described", func(t *testing.T) {
		t.Parallel()

		base := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(base, "/archive.zip", archive, 0o644))

		fsys := &faultFS{FS: base, fileFaults: []string{faultStat}}

		err := vfs.NewZipDecompressor().Unzip(logger.CreateLogger(), fsys, "/dst", "/archive.zip", 0)
		require.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "failed to stat zip archive")
	})

	t.Run("destination that cannot be created", func(t *testing.T) {
		t.Parallel()

		base := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(base, "/archive.zip", archive, 0o644))

		fsys := &faultFS{FS: base, faults: map[string]string{faultMkdirAll: "/dst"}}

		err := vfs.NewZipDecompressor().Unzip(logger.CreateLogger(), fsys, "/dst", "/archive.zip", 0)
		require.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "failed to create directory")
	})

	t.Run("failed closes after a good extraction are only logged", func(t *testing.T) {
		t.Parallel()

		base := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(base, "/archive.zip", archive, 0o644))

		fsys := &faultFS{FS: base, fileFaults: []string{faultClose}}

		var out bytes.Buffer

		l := logger.CreateLogger().WithOptions(log.WithOutput(&out))

		require.NoError(t, vfs.NewZipDecompressor().Unzip(l, fsys, "/dst", "/archive.zip", 0))

		got, err := vfs.ReadFile(base, "/dst/file.txt")
		require.NoError(t, err)
		assert.Equal(t, []byte("content"), got)

		assert.Contains(t, out.String(), "Error closing zip archive")
		assert.Contains(t, out.String(), "Error closing file")
	})
}

func TestUnzipFileSizeLimitExactlyReached(t *testing.T) {
	t.Parallel()

	content := []byte("exactly at the limit")

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(
		fsys, "/archive.zip", createZipArchive(t, map[string][]byte{"file.txt": content}), 0o644,
	))

	z := vfs.NewZipDecompressor(vfs.WithFileSizeLimit(int64(len(content))))

	require.NoError(t, z.Unzip(logger.CreateLogger(), fsys, "/dst", "/archive.zip", 0))

	got, err := vfs.ReadFile(fsys, "/dst/file.txt")
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// rawZipEntry is stored as given, so a test can set any compression method or invalid data.
type rawZipEntry struct {
	name   string
	data   []byte
	mode   os.FileMode
	method uint16
}

// createRawZipArchive writes entries into an archive as they are given.
func createRawZipArchive(t *testing.T, entries ...rawZipEntry) []byte {
	t.Helper()

	var buf bytes.Buffer

	w := zip.NewWriter(&buf)

	for _, entry := range entries {
		header := &zip.FileHeader{
			Name:               entry.name,
			Method:             entry.method,
			CRC32:              crc32.ChecksumIEEE(entry.data),
			CompressedSize64:   uint64(len(entry.data)),
			UncompressedSize64: uint64(len(entry.data)),
		}
		header.SetMode(entry.mode)

		f, err := w.CreateRaw(header)
		require.NoError(t, err)

		_, err = f.Write(entry.data)
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())

	return buf.Bytes()
}
