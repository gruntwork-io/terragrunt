package cas_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestComputeLocalRootHash_Deterministic(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	dir := writeLocalFixture(t, map[string]string{
		"main.tf":          `resource "null_resource" "a" {}`,
		"subdir/nested.tf": `variable "x" {}` + "\n",
		"subdir/README.md": "hello",
	})

	h1, err := c.ComputeLocalRootHash(v, dir, cas.HashSHA256)
	require.NoError(t, err)

	h2, err := c.ComputeLocalRootHash(v, dir, cas.HashSHA256)
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "same directory must produce the same root hash")
	assert.Len(t, h1, 64, "SHA-256 hash should be 64 hex chars")
}

func TestComputeLocalRootHash_DiffersOnContentChange(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	dir := writeLocalFixture(t, map[string]string{
		"main.tf": "one",
	})

	before, err := c.ComputeLocalRootHash(v, dir, cas.HashSHA256)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte("two"), 0o644))

	after, err := c.ComputeLocalRootHash(v, dir, cas.HashSHA256)
	require.NoError(t, err)

	assert.NotEqual(t, before, after, "changing a file's contents must change the root hash")
}

func TestComputeLocalRootHash_DiffersOnModeChange(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("file mode changes are not meaningfully observable on Windows")
	}

	c, v := newCAS(t)
	dir := writeLocalFixture(t, map[string]string{
		"script.sh": "#!/bin/sh\n",
	})

	before, err := c.ComputeLocalRootHash(v, dir, cas.HashSHA256)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(filepath.Join(dir, "script.sh"), 0o755))

	after, err := c.ComputeLocalRootHash(v, dir, cas.HashSHA256)
	require.NoError(t, err)

	assert.NotEqual(t, before, after, "changing a file's mode must change the root hash")
}

func TestComputeLocalRootHash_IgnoresAbsolutePath(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)

	files := map[string]string{
		"main.tf":     `resource "null_resource" "a" {}` + "\n",
		"lib/util.tf": `locals { x = 1 }` + "\n",
	}

	dirA := writeLocalFixture(t, files)
	dirB := writeLocalFixture(t, files)

	hashA, err := c.ComputeLocalRootHash(v, dirA, cas.HashSHA256)
	require.NoError(t, err)

	hashB, err := c.ComputeLocalRootHash(v, dirB, cas.HashSHA256)
	require.NoError(t, err)

	assert.Equal(t, hashA, hashB, "identical contents at different absolute paths must hash identically")
}

// TestStoreLocalDirectoryConcurrentWithRacing pins the blob-then-tree
// write order in storeFetchedContent: a racing reader that finds the
// tree must always find every blob it references. Pre-refactor, the
// original storeLocalContent wrote the tree first and the blobs after,
// leaving a window where a reader could hit a `read_source: failed to
// read file` error when linking blobs. CI runs this test under -race
// per the WithRacing suffix convention.
func TestStoreLocalDirectoryConcurrentWithRacing(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	src := writeLocalFixture(t, map[string]string{
		"main.tf":     `resource "null_resource" "test" {}`,
		"vars.tf":     `variable "x" {}`,
		"README.md":   "readme",
		"sub/nest.tf": "# nested file",
	})

	const n = 8

	dsts := make([]string, n)
	for i := range n {
		dsts[i] = filepath.Join(t.TempDir(), "dst")
	}

	var g errgroup.Group

	for i := range n {
		dst := dsts[i]

		g.Go(func() error {
			return c.StoreLocalDirectory(t.Context(), l, v, src, dst)
		})
	}

	require.NoError(t, g.Wait())

	for _, dst := range dsts {
		require.FileExists(t, filepath.Join(dst, "main.tf"))
		require.FileExists(t, filepath.Join(dst, "sub", "nest.tf"))
	}
}

// TestStoreLocalDirectorySymlink covers symlink ingestion end-to-end: a
// fixture with an in-tree symlink round-trips through StoreLocalDirectory
// and the destination has a real symlink pointing at the same target.
func TestStoreLocalDirectorySymlink(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("os.Symlink on Windows requires special permissions; covered by Unix CI")
	}

	c, v := newCAS(t)
	l := logger.CreateLogger()

	src := writeLocalFixture(t, map[string]string{
		"main.tf": "ok",
	})
	require.NoError(t, os.Symlink("main.tf", filepath.Join(src, "alias.tf")))

	dst := filepath.Join(t.TempDir(), "dst")
	require.NoError(t, c.StoreLocalDirectory(t.Context(), l, v, src, dst))

	info, err := os.Lstat(filepath.Join(dst, "alias.tf"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "alias.tf must be a real symlink")

	got, err := os.Readlink(filepath.Join(dst, "alias.tf"))
	require.NoError(t, err)
	assert.Equal(t, "main.tf", got)
}

// TestStoreLocalDirectoryRejectsEscapingSymlink pins the safety check: a
// symlink whose target climbs above the source root is rejected at ingest
// time rather than poisoning the CAS with content a later materialize would
// have to refuse anyway.
func TestStoreLocalDirectoryRejectsEscapingSymlink(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("os.Symlink on Windows requires special permissions; covered by Unix CI")
	}

	c, v := newCAS(t)
	l := logger.CreateLogger()

	src := writeLocalFixture(t, map[string]string{
		"main.tf": "ok",
	})
	require.NoError(t, os.Symlink("../etc/passwd", filepath.Join(src, "escape")))

	dst := filepath.Join(t.TempDir(), "dst")
	err := c.StoreLocalDirectory(t.Context(), l, v, src, dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink target escapes")
}

// TestComputeLocalRootHashIncludesSymlinks pins that swapping a symlink's
// target changes the root hash. The pre-symlink-support buildLocalTree
// silently skipped symlinks, so two trees that differed only in link target
// hashed identically, breaking the content-addressed contract.
func TestComputeLocalRootHashIncludesSymlinks(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("os.Symlink on Windows requires special permissions; covered by Unix CI")
	}

	c, v := newCAS(t)

	dirA := writeLocalFixture(t, map[string]string{"main.tf": "ok"})
	require.NoError(t, os.Symlink("main.tf", filepath.Join(dirA, "link")))

	dirB := writeLocalFixture(t, map[string]string{"main.tf": "ok"})
	require.NoError(t, os.Symlink("other.tf", filepath.Join(dirB, "link")))

	hashA, err := c.ComputeLocalRootHash(v, dirA, cas.HashSHA256)
	require.NoError(t, err)

	hashB, err := c.ComputeLocalRootHash(v, dirB, cas.HashSHA256)
	require.NoError(t, err)

	assert.NotEqual(t, hashA, hashB, "symlink target must contribute to the root hash")
}

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

// newCAS constructs a CAS instance backed by a fresh per-test store directory
// along with a production [cas.Venv].
func newCAS(t *testing.T) (*cas.CAS, cas.Venv) {
	t.Helper()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	return c, v
}
