package cas_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestFetchSource_MissingBlobIsReIngested(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	const url = "https://example.com/mod.tgz"

	key := cas.OpaqueKey("http", url, "etag-abc")
	resolver := &fakeResolver{scheme: "http", key: key}

	var fetchCalls atomic.Int32

	files := map[string]string{
		"main.tf":  `# hello`,
		"sub/x.tf": `variable "x" {}`,
	}

	dst1 := filepath.Join(t.TempDir(), "dst1")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dst1}, cas.SourceRequest{
		Scheme:   "http",
		URL:      url,
		Resolver: resolver,
		Fetch:    fakeFetcher(c, files, &fetchCalls),
	}))
	require.Equal(t, int32(1), fetchCalls.Load())

	require.NoError(t, v.FS.Remove(storedBlobPath(t, c, v, key, "main.tf")))

	dst2 := filepath.Join(t.TempDir(), "dst2")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dst2}, cas.SourceRequest{
		Scheme:   "http",
		URL:      url,
		Resolver: resolver,
		Fetch:    fakeFetcher(c, files, &fetchCalls),
	}))

	assert.Equal(
		t,
		int32(2),
		fetchCalls.Load(),
		"the cached tree no longer covers the store, so the source must be fetched again",
	)
	assertFileContent(t, v, filepath.Join(dst2, "main.tf"), files["main.tf"])
	assertFileContent(t, v, filepath.Join(dst2, "sub", "x.tf"), files["sub/x.tf"])
}

// TestFetchSource_MissingBlobTheSourceCannotSupply pins what happens when
// re-ingesting does not restore the object: the caller is told which object
// the store is missing, rather than being handed a read failure or a
// directory silently short a file.
func TestFetchSource_MissingBlobTheSourceCannotSupply(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	const url = "https://example.com/mod.tgz"

	key := cas.OpaqueKey("http", url, "etag-abc")
	resolver := &fakeResolver{scheme: "http", key: key}

	var fetchCalls atomic.Int32

	dst1 := filepath.Join(t.TempDir(), "dst1")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dst1}, cas.SourceRequest{
		Scheme:   "http",
		URL:      url,
		Resolver: resolver,
		Fetch:    fakeFetcher(c, map[string]string{"main.tf": "# hello"}, &fetchCalls),
	}))

	blobPath := storedBlobPath(t, c, v, key, "main.tf")
	require.NoError(t, v.FS.Remove(blobPath))

	// A source that answers with the cached key without ingesting anything
	// stands in for one that can no longer produce the content, such as a
	// remote whose artifact has been deleted.
	emptyHanded := func(
		_ context.Context,
		_ log.Logger,
		_ *venv.Venv,
		suggestedKey string,
		_ cas.IngestMode,
	) (string, error) {
		fetchCalls.Add(1)

		return suggestedKey, nil
	}

	dst2 := filepath.Join(t.TempDir(), "dst2")
	err := c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dst2}, cas.SourceRequest{
		Scheme:   "http",
		URL:      url,
		Resolver: resolver,
		Fetch:    emptyHanded,
	})

	var missing *cas.MissingObjectError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, blobPath, missing.Path)
	assert.Equal(t, int32(2), fetchCalls.Load(), "repair is one attempt, not a retry loop")
}

// TestFetchSource_ConcurrentRepairWithRacing has every worker find the same
// blob missing at once, so they race to restore it. CI runs this test under
// -race per the WithRacing suffix convention.
func TestFetchSource_ConcurrentRepairWithRacing(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	const url = "https://example.com/mod.tgz"

	key := cas.OpaqueKey("http", url, "etag-abc")
	resolver := &fakeResolver{scheme: "http", key: key}

	var fetchCalls atomic.Int32

	files := map[string]string{
		"main.tf":   `# hello`,
		"vars.tf":   `variable "x" {}`,
		"sub/y.tf":  `# nested`,
		"README.md": "readme",
	}

	dst := filepath.Join(t.TempDir(), "dst")
	require.NoError(t, c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dst}, cas.SourceRequest{
		Scheme:   "http",
		URL:      url,
		Resolver: resolver,
		Fetch:    fakeFetcher(c, files, &fetchCalls),
	}))

	require.NoError(t, v.FS.Remove(storedBlobPath(t, c, v, key, "main.tf")))

	const workers = 8

	dsts := make([]string, workers)
	for i := range workers {
		dsts[i] = filepath.Join(t.TempDir(), "dst")
	}

	var g errgroup.Group

	for i := range workers {
		g.Go(func() error {
			return c.FetchSource(t.Context(), l, v, &cas.CloneOptions{Dir: dsts[i]}, cas.SourceRequest{
				Scheme:   "http",
				URL:      url,
				Resolver: resolver,
				Fetch:    fakeFetcher(c, files, &fetchCalls),
			})
		})
	}

	require.NoError(t, g.Wait())

	for _, dir := range dsts {
		assertFileContent(t, v, filepath.Join(dir, "main.tf"), files["main.tf"])
		assertFileContent(t, v, filepath.Join(dir, "sub", "y.tf"), files["sub/y.tf"])
	}
}

// TestMaterializeTree_MissingBlob covers the entry point that has no source
// to go back to. A cas:: reference names a tree and nothing else, so the
// miss can only be reported.
func TestMaterializeTree_MissingBlob(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	src := writeLocalFixture(t, map[string]string{"main.tf": "# hello"})

	key, err := c.IngestDirectory(l, v, src, "")
	require.NoError(t, err)

	blobPath := storedBlobPath(t, c, v, key, "main.tf")
	require.NoError(t, v.FS.Remove(blobPath))

	dst := filepath.Join(t.TempDir(), "dst")

	var missing *cas.MissingObjectError
	require.ErrorAs(t, c.MaterializeTree(t.Context(), l, v, key, dst), &missing)
	assert.Equal(t, blobPath, missing.Path)
}

func TestCASClone_E2E_MissingBlobIsReIngested(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)
	headHash := resolveHeadE2E(t, repoURL)

	tempDir := helpers.TmpDirWOSymlinks(t)
	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	dst1 := filepath.Join(tempDir, "dst1")
	require.NoError(t, c.Clone(t.Context(), l, v, repoURL,
		cas.WithDir(dst1),
		cas.WithBranch("main"),
		cas.WithDepth(-1)))

	want := readFile(t, v, filepath.Join(dst1, "main.tf"))

	require.NoError(t, v.FS.Remove(storedBlobPath(t, c, v, headHash, "main.tf")))

	// The tree is still stored under the commit SHA, so this clone takes the
	// probe hit that skips the fetch entirely until the miss turns up.
	dst2 := filepath.Join(tempDir, "dst2")
	require.NoError(t, c.Clone(t.Context(), l, v, repoURL,
		cas.WithDir(dst2),
		cas.WithBranch("main"),
		cas.WithDepth(-1)))

	assertFileContent(t, v, filepath.Join(dst2, "main.tf"), want)
	require.FileExists(t, filepath.Join(dst2, "README.md"))
}

// TestCASClone_E2E_RepairKeepsIncludedGitFilesOnce pins that re-ingesting a
// tree whose included .git files are already recorded does not list any of
// them twice: the re-ingest writes records of its own for names not yet
// recorded, and the stored tree never carries a .git entry.
func TestCASClone_E2E_RepairKeepsIncludedGitFilesOnce(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)
	headHash := resolveHeadE2E(t, repoURL)

	tempDir := helpers.TmpDirWOSymlinks(t)
	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()
	l := logger.CreateLogger()

	clone := func(dst string) {
		t.Helper()

		require.NoError(t, c.Clone(t.Context(), l, v, repoURL,
			cas.WithDir(dst),
			cas.WithBranch("main"),
			cas.WithIncludedGitFiles([]string{"HEAD", "config"}),
			cas.WithDepth(-1)))
	}

	clone(filepath.Join(tempDir, "dst1"))

	require.NoError(t, v.FS.Remove(storedBlobPath(t, c, v, headHash, "main.tf")))

	dst2 := filepath.Join(tempDir, "dst2")
	clone(dst2)

	require.FileExists(t, filepath.Join(dst2, ".git", "HEAD"))
	require.FileExists(t, filepath.Join(dst2, ".git", "config"))

	// The repair re-ingest records the .git files it lists against the
	// commit, one record per name, and never folds them into the stored
	// tree: the listing it re-reads cannot double as a place to re-append.
	treeData, err := cas.NewContent(c.TreeStore()).Read(v, headHash)
	require.NoError(t, err)

	assert.NotContains(t, string(treeData), "\t.git/HEAD\n")
	assert.NotContains(t, string(treeData), "\t.git/config\n")

	records := cas.NewContent(c.GitFileStore())

	for _, name := range []string{"HEAD", "config"} {
		record, readErr := records.Read(v, cas.GitFileKey(headHash, name))
		require.NoError(t, readErr)
		assert.Equal(t, 1, strings.Count(string(record), "\t"+name+"\n"))
	}
}

// storedTree parses the tree the store holds under treeKey.
func storedTree(t *testing.T, c *cas.CAS, v *venv.Venv, treeKey string) *git.Tree {
	t.Helper()

	data, err := cas.NewContent(c.TreeStore()).Read(v, treeKey)
	require.NoError(t, err)

	tree, err := git.ParseTree(data, "")
	require.NoError(t, err)

	return tree
}

// storedBlobPath returns where the store keeps the blob for relPath in the
// tree stored under treeKey.
func storedBlobPath(t *testing.T, c *cas.CAS, v *venv.Venv, treeKey, relPath string) string {
	t.Helper()

	for _, entry := range storedTree(t, c, v, treeKey).Entries() {
		if entry.Path == relPath && entry.Type == git.EntryTypeBlob {
			return filepath.Join(c.BlobStore().Path(), entry.Hash[:2], entry.Hash)
		}
	}

	t.Fatalf("tree %s holds no blob for %s", treeKey, relPath)

	return ""
}

func readFile(t *testing.T, v *venv.Venv, path string) string {
	t.Helper()

	data, err := vfs.ReadFile(v.FS, path)
	require.NoError(t, err)

	return string(data)
}

func assertFileContent(t *testing.T, v *venv.Venv, path, want string) {
	t.Helper()

	data, err := vfs.ReadFile(v.FS, path)
	require.NoError(t, err)
	assert.Equal(t, want, string(data))
}
