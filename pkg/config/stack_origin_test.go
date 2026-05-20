package config_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testStackCopyDir = "/abs/copy"
	stackOriginFile  = ".terragrunt-stack-origin"
)

// TestReadStackOrigin_AcceptsAbsoluteExistingDir pins the happy path: a valid sidecar pointing at an absolute existing directory returns that directory.
func TestReadStackOrigin_AcceptsAbsoluteExistingDir(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	catalog := "/abs/catalog"
	require.NoError(t, fsys.MkdirAll(catalog, 0755))

	require.NoError(t, fsys.MkdirAll(testStackCopyDir, 0755))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(testStackCopyDir, stackOriginFile), []byte(catalog+"\n"), 0644))

	got := config.ReadStackOrigin(fsys, testStackCopyDir)
	assert.Equal(t, catalog, got)
}

// TestReadStackOrigin_RejectsNonAbsolute pins that a relative path in the sidecar is rejected (returns "") so a hand-edited or malformed sidecar cannot redirect resolution to a working-dir-relative location.
func TestReadStackOrigin_RejectsNonAbsolute(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, fsys.MkdirAll(testStackCopyDir, 0755))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(testStackCopyDir, stackOriginFile), []byte("relative/path"), 0644))

	assert.Empty(t, config.ReadStackOrigin(fsys, testStackCopyDir))
}

// TestReadStackOrigin_RejectsNonExistent pins that a sidecar pointing to a non-existent path is rejected (returns "") so a stale workspace cannot redirect resolution.
func TestReadStackOrigin_RejectsNonExistent(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, fsys.MkdirAll(testStackCopyDir, 0755))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(testStackCopyDir, stackOriginFile), []byte("/nonexistent/path/that/should/never/exist"), 0644))

	assert.Empty(t, config.ReadStackOrigin(fsys, testStackCopyDir))
}

// TestReadStackOrigin_RejectsFile pins that a sidecar pointing at a regular file (not a directory) is rejected.
func TestReadStackOrigin_RejectsFile(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	regularFile := "/abs/not-a-dir"
	require.NoError(t, vfs.WriteFile(fsys, regularFile, []byte("hi"), 0644))

	require.NoError(t, fsys.MkdirAll(testStackCopyDir, 0755))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(testStackCopyDir, stackOriginFile), []byte(regularFile), 0644))

	assert.Empty(t, config.ReadStackOrigin(fsys, testStackCopyDir))
}

// TestReadStackOrigin_RejectsEmpty pins that an empty (or whitespace-only) sidecar is rejected.
func TestReadStackOrigin_RejectsEmpty(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, fsys.MkdirAll(testStackCopyDir, 0755))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(testStackCopyDir, stackOriginFile), []byte("   \n  "), 0644))

	assert.Empty(t, config.ReadStackOrigin(fsys, testStackCopyDir))
}

// TestReadStackOrigin_AbsentSidecar pins that a directory without the sidecar returns "" cleanly.
func TestReadStackOrigin_AbsentSidecar(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, fsys.MkdirAll(testStackCopyDir, 0755))

	assert.Empty(t, config.ReadStackOrigin(fsys, testStackCopyDir))
}

// TestReadStackOrigin_NormalizesPath pins that the returned path is filepath.Clean'd (drops redundant separators and "." segments).
func TestReadStackOrigin_NormalizesPath(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	catalog := "/abs/catalog"
	require.NoError(t, fsys.MkdirAll(catalog, 0755))

	require.NoError(t, fsys.MkdirAll(testStackCopyDir, 0755))

	noisy := "/abs/./catalog/"
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(testStackCopyDir, stackOriginFile), []byte(noisy), 0644))

	assert.Equal(t, catalog, config.ReadStackOrigin(fsys, testStackCopyDir))
}
