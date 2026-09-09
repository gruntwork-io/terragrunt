package git_test

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const extractWriters = 4

type archiveEntry struct {
	name     string
	body     string
	link     string
	mode     int64
	typeflag byte
}

func TestExtractArchive(t *testing.T) {
	t.Parallel()

	// A file large enough to be written straight from the stream rather than
	// buffered for a worker.
	largeBody := strings.Repeat("terragrunt", 200_000)

	testCases := []struct {
		wantFiles map[string]string
		wantErr   error
		name      string
		entries   []archiveEntry
	}{
		{
			name: "files and directories",
			entries: []archiveEntry{
				{name: "unit/", typeflag: tar.TypeDir, mode: 0o755},
				{name: "unit/terragrunt.hcl", body: "inputs = {}\n", mode: 0o644},
				{name: "unit/run.sh", body: "#!/bin/sh\n", mode: 0o755},
			},
			wantFiles: map[string]string{
				"unit/terragrunt.hcl": "inputs = {}\n",
				"unit/run.sh":         "#!/bin/sh\n",
			},
		},
		{
			name: "entry naming the root of the archive",
			entries: []archiveEntry{
				{name: "./", typeflag: tar.TypeDir, mode: 0o755},
				{name: "./terragrunt.hcl", body: "inputs = {}\n", mode: 0o644},
			},
			wantFiles: map[string]string{"terragrunt.hcl": "inputs = {}\n"},
		},
		{
			name: "global pax header is not a file",
			entries: []archiveEntry{
				{name: "pax_global_header", body: "52 comment=abc\n", typeflag: tar.TypeXGlobalHeader},
				{name: "terragrunt.hcl", body: "inputs = {}\n", mode: 0o644},
			},
			wantFiles: map[string]string{"terragrunt.hcl": "inputs = {}\n"},
		},
		{
			name: "content larger than the buffering threshold",
			entries: []archiveEntry{
				{name: "big.json", body: largeBody, mode: 0o644},
			},
			wantFiles: map[string]string{"big.json": largeBody},
		},
		{
			name: "entry climbing out of the destination",
			entries: []archiveEntry{
				{name: "../escaped.hcl", body: "inputs = {}\n", mode: 0o644},
			},
			wantErr: git.ErrArchiveEntryOutsideDest,
		},
		{
			name: "absolute entry",
			entries: []archiveEntry{
				{name: "/etc/passwd", body: "root\n", mode: 0o644},
			},
			wantErr: git.ErrArchiveEntryOutsideDest,
		},
		{
			name: "entry nested past the depth bound",
			entries: []archiveEntry{
				{name: strings.Repeat("nested/", 64) + "terragrunt.hcl", body: "inputs = {}\n", mode: 0o644},
			},
			wantErr: git.ErrArchiveTooDeep,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dest := helpers.TmpDirWOSymlinks(t)

			err := git.ExtractArchive(
				t.Context(),
				venvtest.NewWithOSFS(),
				bytes.NewReader(buildArchive(t, tc.entries)),
				dest,
				extractWriters,
			)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)

				return
			}

			require.NoError(t, err)

			for name, want := range tc.wantFiles {
				got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name)))
				require.NoError(t, err)
				assert.Equal(t, want, string(got))
			}
		})
	}
}

func TestExtractArchiveSymlinks(t *testing.T) {
	t.Parallel()

	t.Run("target inside the destination", func(t *testing.T) {
		t.Parallel()

		dest := helpers.TmpDirWOSymlinks(t)

		err := git.ExtractArchive(
			t.Context(),
			venvtest.NewWithOSFS(),
			bytes.NewReader(buildArchive(t, []archiveEntry{
				{name: "common.hcl", body: "inputs = {}\n", mode: 0o644},
				{name: "unit/shared.hcl", link: "../common.hcl", typeflag: tar.TypeSymlink},
			})),
			dest,
			extractWriters,
		)
		require.NoError(t, err)

		target, err := os.Readlink(filepath.Join(dest, "unit", "shared.hcl"))
		require.NoError(t, err)
		assert.Equal(t, "../common.hcl", target)
	})

	t.Run("target outside the destination", func(t *testing.T) {
		t.Parallel()

		dest := helpers.TmpDirWOSymlinks(t)

		err := git.ExtractArchive(
			t.Context(),
			venvtest.NewWithOSFS(),
			bytes.NewReader(buildArchive(t, []archiveEntry{
				{name: "unit/escaped.hcl", link: "../../outside.hcl", typeflag: tar.TypeSymlink},
			})),
			dest,
			extractWriters,
		)
		require.ErrorIs(t, err, vfs.ErrSymlinkEscapes)
	})
}

func TestExtractArchiveTruncatedStream(t *testing.T) {
	t.Parallel()

	archive := buildArchive(t, []archiveEntry{
		{name: "terragrunt.hcl", body: strings.Repeat("inputs = {}\n", 1000), mode: 0o644},
	})

	// Cut the stream inside the entry's content, as a git process killed
	// mid-archive would.
	err := git.ExtractArchive(
		t.Context(),
		venvtest.NewWithOSFS(),
		bytes.NewReader(archive[:1024]),
		helpers.TmpDirWOSymlinks(t),
		extractWriters,
	)
	require.Error(t, err)
}

// buildArchive writes entries into an uncompressed tar stream shaped like the
// output of `git archive`.
func buildArchive(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()

	var buf bytes.Buffer

	writer := tar.NewWriter(&buf)

	for _, entry := range entries {
		if entry.typeflag == tar.TypeXGlobalHeader {
			// Git leads its archives with one of these, and the tar writer
			// accepts it only as a bare set of pax records.
			require.NoError(t, writer.WriteHeader(&tar.Header{
				Name:       entry.name,
				Typeflag:   tar.TypeXGlobalHeader,
				PAXRecords: map[string]string{"comment": entry.body},
			}))

			continue
		}

		header := &tar.Header{
			Name:     entry.name,
			Mode:     entry.mode,
			Size:     int64(len(entry.body)),
			Typeflag: entry.typeflag,
			Linkname: entry.link,
		}

		if header.Typeflag == 0 {
			header.Typeflag = tar.TypeReg
		}

		if header.Typeflag == tar.TypeDir || header.Typeflag == tar.TypeSymlink {
			header.Size = 0
		}

		require.NoError(t, writer.WriteHeader(header))

		if header.Size > 0 {
			_, err := writer.Write([]byte(entry.body))
			require.NoError(t, err)
		}
	}

	require.NoError(t, writer.Close())

	return buf.Bytes()
}
