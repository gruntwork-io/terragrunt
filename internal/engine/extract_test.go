package engine

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtract_ZipSlipPathTraversal verifies that extract rejects ZIP archives
// containing entries that traverse outside the target directory (ZipSlip attack).
func TestExtract_ZipSlipPathTraversal(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	zipPath := filepath.Join(t.TempDir(), "malicious.zip")

	// Create a ZIP archive with a path traversal entry
	f, err := os.Create(zipPath)
	require.NoError(t, err)

	w := zip.NewWriter(f)
	// Entry that escapes the target directory
	_, err = w.Create("../../etc/malicious")
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	l := log.New()
	err = extract(l, zipPath, destDir)
	require.Error(t, err, "extract should reject ZIP with path traversal entries")
	assert.True(t, strings.Contains(err.Error(), "path traversal"),
		"error should mention path traversal, got: %v", err)
}

// TestExtract_ValidZip verifies that extract correctly extracts a well-formed
// ZIP archive and produces the expected files on disk.
func TestExtract_ValidZip(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	zipPath := filepath.Join(t.TempDir(), "valid.zip")

	f, err := os.Create(zipPath)
	require.NoError(t, err)

	w := zip.NewWriter(f)
	writer, err := w.Create("subdir/hello.txt")
	require.NoError(t, err)
	_, err = writer.Write([]byte("hello world"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	l := log.New()
	err = extract(l, zipPath, destDir)
	require.NoError(t, err, "extract should succeed for valid ZIP")

	data, err := os.ReadFile(filepath.Join(destDir, "subdir", "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}
