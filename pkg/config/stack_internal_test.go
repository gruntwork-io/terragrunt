package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadStackOrigin_AcceptsAbsoluteExistingDir pins the happy path: a valid sidecar pointing at an absolute existing directory returns that directory.
func TestReadStackOrigin_AcceptsAbsoluteExistingDir(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	catalog := filepath.Join(tmp, "catalog")
	require.NoError(t, os.MkdirAll(catalog, 0755))

	stackDir := filepath.Join(tmp, "copy")
	require.NoError(t, os.MkdirAll(stackDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, stackOriginFile), []byte(catalog+"\n"), 0644))

	got := readStackOrigin(stackDir)
	assert.Equal(t, catalog, got)
}

// TestReadStackOrigin_RejectsNonAbsolute pins that a relative path in the sidecar is rejected (returns "") so a hand-edited or malformed sidecar cannot redirect resolution to a working-dir-relative location.
func TestReadStackOrigin_RejectsNonAbsolute(t *testing.T) {
	t.Parallel()

	stackDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, stackOriginFile), []byte("relative/path"), 0644))

	assert.Empty(t, readStackOrigin(stackDir))
}

// TestReadStackOrigin_RejectsNonExistent pins that a sidecar pointing to a non-existent path is rejected (returns "") so a stale workspace cannot redirect resolution.
func TestReadStackOrigin_RejectsNonExistent(t *testing.T) {
	t.Parallel()

	stackDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, stackOriginFile), []byte("/nonexistent/path/that/should/never/exist"), 0644))

	assert.Empty(t, readStackOrigin(stackDir))
}

// TestReadStackOrigin_RejectsFile pins that a sidecar pointing at a regular file (not a directory) is rejected.
func TestReadStackOrigin_RejectsFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	regularFile := filepath.Join(tmp, "not-a-dir")
	require.NoError(t, os.WriteFile(regularFile, []byte("hi"), 0644))

	stackDir := filepath.Join(tmp, "copy")
	require.NoError(t, os.MkdirAll(stackDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, stackOriginFile), []byte(regularFile), 0644))

	assert.Empty(t, readStackOrigin(stackDir))
}

// TestReadStackOrigin_RejectsEmpty pins that an empty (or whitespace-only) sidecar is rejected.
func TestReadStackOrigin_RejectsEmpty(t *testing.T) {
	t.Parallel()

	stackDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, stackOriginFile), []byte("   \n  "), 0644))

	assert.Empty(t, readStackOrigin(stackDir))
}

// TestReadStackOrigin_AbsentSidecar pins that a directory without the sidecar returns "" cleanly.
func TestReadStackOrigin_AbsentSidecar(t *testing.T) {
	t.Parallel()
	assert.Empty(t, readStackOrigin(t.TempDir()))
}

// TestReadStackOrigin_NormalizesPath pins that the returned path is filepath.Clean'd (drops redundant separators and "." segments).
func TestReadStackOrigin_NormalizesPath(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	catalog := filepath.Join(tmp, "catalog")
	require.NoError(t, os.MkdirAll(catalog, 0755))

	stackDir := filepath.Join(tmp, "copy")
	require.NoError(t, os.MkdirAll(stackDir, 0755))

	noisy := filepath.Join(tmp, ".", "catalog", "")
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, stackOriginFile), []byte(noisy), 0644))

	assert.Equal(t, catalog, readStackOrigin(stackDir))
}
