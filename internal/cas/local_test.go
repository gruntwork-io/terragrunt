package cas_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeLocalFixture populates a fresh directory with a small deterministic tree
// and returns its absolute path.
func writeLocalFixture(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := helpers.TmpDirWOSymlinks(t)
	for rel, body := range files {
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}

	return dir
}

// newCAS constructs a CAS instance backed by a fresh per-test store directory.
func newCAS(t *testing.T) *cas.CAS {
	t.Helper()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	return c
}

func TestComputeLocalRootHash_Deterministic(t *testing.T) {
	t.Parallel()

	c := newCAS(t)
	dir := writeLocalFixture(t, map[string]string{
		"main.tf":          `resource "null_resource" "a" {}`,
		"subdir/nested.tf": `variable "x" {}` + "\n",
		"subdir/README.md": "hello",
	})

	h1, err := c.ComputeLocalRootHash(dir, cas.HashSHA256)
	require.NoError(t, err)

	h2, err := c.ComputeLocalRootHash(dir, cas.HashSHA256)
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "same directory must produce the same root hash")
	assert.Len(t, h1, 64, "SHA-256 hash should be 64 hex chars")
}

func TestComputeLocalRootHash_DiffersOnContentChange(t *testing.T) {
	t.Parallel()

	c := newCAS(t)
	dir := writeLocalFixture(t, map[string]string{
		"main.tf": "one",
	})

	before, err := c.ComputeLocalRootHash(dir, cas.HashSHA256)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte("two"), 0o644))

	after, err := c.ComputeLocalRootHash(dir, cas.HashSHA256)
	require.NoError(t, err)

	assert.NotEqual(t, before, after, "changing a file's contents must change the root hash")
}

func TestComputeLocalRootHash_DiffersOnModeChange(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("file mode changes are not meaningfully observable on Windows")
	}

	c := newCAS(t)
	dir := writeLocalFixture(t, map[string]string{
		"script.sh": "#!/bin/sh\n",
	})

	before, err := c.ComputeLocalRootHash(dir, cas.HashSHA256)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(filepath.Join(dir, "script.sh"), 0o755))

	after, err := c.ComputeLocalRootHash(dir, cas.HashSHA256)
	require.NoError(t, err)

	assert.NotEqual(t, before, after, "changing a file's mode must change the root hash")
}

func TestComputeLocalRootHash_IgnoresAbsolutePath(t *testing.T) {
	t.Parallel()

	c := newCAS(t)

	files := map[string]string{
		"main.tf":     `resource "null_resource" "a" {}` + "\n",
		"lib/util.tf": `locals { x = 1 }` + "\n",
	}

	dirA := writeLocalFixture(t, files)
	dirB := writeLocalFixture(t, files)

	hashA, err := c.ComputeLocalRootHash(dirA, cas.HashSHA256)
	require.NoError(t, err)

	hashB, err := c.ComputeLocalRootHash(dirB, cas.HashSHA256)
	require.NoError(t, err)

	assert.Equal(t, hashA, hashB, "identical contents at different absolute paths must hash identically")
}
