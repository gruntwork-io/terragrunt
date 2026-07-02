package engine

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractArchiveWithLimitsAcceptsSmallArchive(t *testing.T) {
	t.Parallel()

	baseDir := newArchiveTestDir(t)
	archivePath := filepath.Join(baseDir, "engine.zip")
	engineFile := filepath.Join(baseDir, "engine-bin")

	writeZipArchive(t, archivePath, []zipArchiveEntry{
		{
			name: "engine",
			body: []byte("engine-content"),
		},
	})

	err := extractArchiveWithLimits(logger.CreateLogger(), archivePath, engineFile, 1024, 2)
	require.NoError(t, err)

	content, err := os.ReadFile(engineFile)
	require.NoError(t, err)
	assert.Equal(t, []byte("engine-content"), content)
}

func TestExtractArchiveWithLimitsAcceptsExactSizeLimit(t *testing.T) {
	t.Parallel()

	baseDir := newArchiveTestDir(t)
	archivePath := filepath.Join(baseDir, "engine.zip")
	engineFile := filepath.Join(baseDir, "engine-bin")

	writeZipArchive(t, archivePath, []zipArchiveEntry{
		{
			name: "engine",
			body: []byte("01234567"),
		},
	})

	err := extractArchiveWithLimits(logger.CreateLogger(), archivePath, engineFile, 8, 2)
	require.NoError(t, err)

	content, err := os.ReadFile(engineFile)
	require.NoError(t, err)
	assert.Equal(t, []byte("01234567"), content)
}

func TestExtractArchiveWithLimitsRejectsTooManyFiles(t *testing.T) {
	t.Parallel()

	baseDir := newArchiveTestDir(t)
	archivePath := filepath.Join(baseDir, "engine.zip")
	engineFile := filepath.Join(baseDir, "engine-bin")

	writeZipArchive(t, archivePath, []zipArchiveEntry{
		{
			name: "one",
			body: []byte("1"),
		},
		{
			name: "two",
			body: []byte("2"),
		},
		{
			name: "three",
			body: []byte("3"),
		},
	})

	err := extractArchiveWithLimits(logger.CreateLogger(), archivePath, engineFile, 1024, 2)

	var extractionErr *archiveExtractionError
	require.ErrorAs(t, err, &extractionErr)
	assert.Equal(t, archivePath, extractionErr.downloadFile)
	assert.NoFileExists(t, engineFile)
	assert.NoFileExists(t, filepath.Join(baseDir, "one"))
	assert.NoFileExists(t, filepath.Join(baseDir, "two"))
	assert.NoFileExists(t, filepath.Join(baseDir, "three"))
}

func TestExtractArchiveWithLimitsRejectsOversizedContent(t *testing.T) {
	t.Parallel()

	baseDir := newArchiveTestDir(t)
	archivePath := filepath.Join(baseDir, "engine.zip")
	engineFile := filepath.Join(baseDir, "engine-bin")

	writeZipArchive(t, archivePath, []zipArchiveEntry{
		{
			name: "engine",
			body: []byte("0123456789abcdef"),
		},
	})

	err := extractArchiveWithLimits(logger.CreateLogger(), archivePath, engineFile, 8, 2)

	var extractionErr *archiveExtractionError
	require.ErrorAs(t, err, &extractionErr)
	assert.Equal(t, archivePath, extractionErr.downloadFile)
	assert.NoFileExists(t, engineFile)
}

type zipArchiveEntry struct {
	name string
	body []byte
}

func newArchiveTestDir(t *testing.T) string {
	t.Helper()

	// NOTE: extractArchiveWithLimits uses os.Rename, os.MkdirTemp, and vfs.NewOSFS.
	return t.TempDir()
}

func writeZipArchive(t *testing.T, archivePath string, entries []zipArchiveEntry) {
	t.Helper()

	archiveFile, err := os.Create(archivePath)
	require.NoError(t, err)

	zipWriter := zip.NewWriter(archiveFile)

	for _, entry := range entries {
		writer, err := zipWriter.Create(entry.name)
		require.NoError(t, err)

		_, err = writer.Write(entry.body)
		require.NoError(t, err)
	}

	require.NoError(t, zipWriter.Close())
	require.NoError(t, archiveFile.Close())
}
