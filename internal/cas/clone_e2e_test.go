package cas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestCASClone_E2E_SymbolicRefSecondRunReusesCache(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)
	headHash := resolveHeadE2E(t, repoURL)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")
	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venv.OSVenv()

	l := logger.CreateLogger()

	// First clone: probe hits, fetcher runs (tree not cached yet).
	dst1 := filepath.Join(tempDir, "dst1")
	require.NoError(t, c.Clone(t.Context(), l, v, repoURL, cas.WithDir(dst1),
		cas.WithBranch("main"),
		cas.WithDepth(-1)))

	require.FileExists(t, filepath.Join(dst1, "README.md"))
	require.FileExists(t, filepath.Join(dst1, "main.tf"))

	// Tree is stored under the commit SHA, the same key the second
	// clone's probe will derive.
	treeContent := cas.NewContent(c.TreeStore())
	_, err = treeContent.Read(v, headHash)
	require.NoError(t, err, "tree must be stored under the canonical commit SHA")

	// Second clone: probe still hits ls-remote, derives the same key,
	// FetchSource short-circuits via treeStore.NeedsWrite, fetcher
	// never runs.
	dst2 := filepath.Join(tempDir, "dst2")
	require.NoError(t, c.Clone(t.Context(), l, v, repoURL, cas.WithDir(dst2),
		cas.WithBranch("main"),
		cas.WithDepth(-1)))

	require.FileExists(t, filepath.Join(dst2, "README.md"))
	require.FileExists(t, filepath.Join(dst2, "main.tf"))
}

func TestCASClone_E2E_CommitFormRefRoundTrip(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)
	headHash := resolveHeadE2E(t, repoURL)

	tempDir := helpers.TmpDirWOSymlinks(t)
	c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	v := venv.OSVenv()

	l := logger.CreateLogger()

	// Probe will return ErrNoVersionMetadata (ls-remote can't resolve
	// a raw SHA), so fetcher canonicalizes via populateTreeFromCommitRef.
	dst := filepath.Join(tempDir, "dst")
	require.NoError(t, c.Clone(t.Context(), l, v, repoURL, cas.WithDir(dst),
		cas.WithBranch(headHash),
		cas.WithDepth(-1)))

	require.FileExists(t, filepath.Join(dst, "README.md"))

	// The canonical SHA path stores the tree under the resolved commit
	// hash, so a follow-up symbolic clone of "main" finds it.
	treeContent := cas.NewContent(c.TreeStore())
	_, err = treeContent.Read(v, headHash)
	require.NoError(t, err)
}

func TestCASClone_E2E_ThroughCASGetter(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)

	tempDir := helpers.TmpDirWOSymlinks(t)
	c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	v := venv.OSVenv()

	l := logger.CreateLogger()

	// Full CASGetter dispatch (Detect → Get → CAS.Clone). The
	// CASGetter is responsible for the ?ref= round-trip.
	g := getter.NewCASGetter(l, c, v, &cas.CloneOptions{Depth: -1})
	client := &getter.Client{Getters: []getter.Getter{g}}

	dst := filepath.Join(tempDir, "dst")
	_, err = client.Get(t.Context(), &getter.Request{
		Src:     "git::" + repoURL + "?ref=main",
		Dst:     dst,
		GetMode: getter.ModeDir,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dst, "README.md"))
}

func TestCASClone_E2E_RemainsOfflineAfterFirstClone(t *testing.T) {
	t.Parallel()

	// Once the tree is cached, a second Clone() keyed by the full
	// commit SHA resolves entirely from the local CAS:
	//   - GitResolver.Probe sees looksLikeFullSHA(Branch) and runs
	//     ProbeCachedCommit against the bare repo (rev-parse, no
	//     network).
	//   - FetchSource finds the tree already stored under that SHA
	//     and short-circuits to linkStoredTree; Fetch never runs.
	//
	// We pin both halves by shutting the server down between clones:
	// the second Clone() must succeed, and (since LsRemote would
	// fail-fast against a dead listener) any path that still reaches
	// it would surface here.
	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile(t.Context(), "README.md", []byte("hi"), "init"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	headHash, err := srv.Head(t.Context())
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	v := venv.OSVenv()

	l := logger.CreateLogger()

	dst1 := filepath.Join(tempDir, "dst1")
	require.NoError(t, c.Clone(t.Context(), l, v, repoURL, cas.WithDir(dst1),
		cas.WithBranch("main"),
		cas.WithDepth(-1)))

	// Shut the server down. Any subsequent ls-remote against repoURL
	// would fail with "Could not resolve host" / "Connection refused".
	require.NoError(t, srv.Close())

	dst2 := filepath.Join(tempDir, "dst2")
	require.NoError(t, c.Clone(
		t.Context(),
		l,
		v,
		repoURL,
		cas.WithDir(dst2),
		cas.WithBranch(headHash),
		cas.WithDepth(
			-1,
		),
	), "second clone keyed by full SHA must resolve from local CAS without ls-remote")

	require.FileExists(t, filepath.Join(dst2, "README.md"))
}

func TestCASClone_E2E_MutableSetCopiesBlobs(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)

	tempDir := helpers.TmpDirWOSymlinks(t)
	c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	v := venv.OSVenv()

	l := logger.CreateLogger()

	dst := filepath.Join(tempDir, "dst")
	require.NoError(t, c.Clone(t.Context(), l, v, repoURL, cas.WithDir(dst),
		cas.WithBranch("main"),
		cas.WithDepth(-1),
		cas.WithMutable(true)))

	// Mutable=true: destination files have the original perms, not
	// the write-bit-stripped read-only perms the default path uses.
	stat, err := os.Stat(filepath.Join(dst, "README.md"))
	require.NoError(t, err)
	assert.NotEqual(t, os.FileMode(0o444), stat.Mode().Perm(),
		"mutable clone should not strip write bits; default path does")
}

// TestCASClone_E2E_DepthQueryParamWithTag reproduces #6512: a
// terraform.source that combines the go-getter depth query parameter with
// ref=<tag>. Before the fix the CAS getter left "?depth=1" in the URL handed
// to git, which rejected it as part of the repository name. The tagged
// commit is deliberately behind HEAD so a shallow clone of the default
// branch would not contain it, exercising the --branch <tag> --depth path.
func TestCASClone_E2E_DepthQueryParamWithTag(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)

	require.NoError(t, srv.CommitFile(t.Context(), "README.md", []byte("# test repo"), "add readme"))
	require.NoError(t, srv.CommitFile(t.Context(), "main.tf", []byte("# tagged"), "tagged content"))
	require.NoError(t, srv.Tag(t.Context(), "v1.0.0"))
	// Advance the default branch past the tag.
	require.NoError(t, srv.CommitFile(t.Context(), "main.tf", []byte("# newer"), "post-tag commit"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	v := venv.OSVenv()
	l := logger.CreateLogger()

	// Depth left at the CAS default so ?depth=1 in the URL is what drives the
	// shallow clone.
	g := getter.NewCASGetter(l, c, v, &cas.CloneOptions{})
	client := &getter.Client{Getters: []getter.Getter{g}}

	dst := filepath.Join(tempDir, "dst")
	_, err = client.Get(t.Context(), &getter.Request{
		Src:     "git::" + repoURL + "?depth=1&ref=v1.0.0",
		Dst:     dst,
		GetMode: getter.ModeDir,
	})
	require.NoError(t, err)

	// The checked-out content must be the tagged commit, not HEAD.
	content, err := os.ReadFile(filepath.Join(dst, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# tagged", string(content))
}

// TestCASClone_E2E_URLDepthOverridesAmbientFullHistory pins the headline
// behavior of #6512's fix: a depth on the source URL overrides the ambient CAS
// clone depth. Its sibling TestCASClone_E2E_DepthQueryParamWithTag leaves the
// ambient at the CAS default of 1, where ?depth=1 is indistinguishable from no
// depth at all — that test would pass even if depth were parsed and thrown
// away. Here the ambient is full history (WithCloneDepth(-1)), so ?depth=1 can
// only produce a shallow fetch if the URL value actually took effect. The
// shallow fetch is observed via the `shallow` marker git writes into the
// central store's bare repo on a --depth fetch.
func TestCASClone_E2E_URLDepthOverridesAmbientFullHistory(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)

	require.NoError(t, srv.CommitFile(t.Context(), "README.md", []byte("# test repo"), "add readme"))
	require.NoError(t, srv.CommitFile(t.Context(), "main.tf", []byte("# tagged"), "tagged content"))
	require.NoError(t, srv.Tag(t.Context(), "v1.0.0"))
	// The tagged commit already has the README commit as its parent, so a
	// --depth 1 fetch of the tag truncates that ancestor and git writes the
	// `shallow` marker this test asserts on. This extra commit advances main
	// past the tag so the tag sits behind HEAD, which is what makes the
	// "# tagged" content assertion below discriminating: without it HEAD would
	// equal the tag and a wrong-ref checkout could not be detected.
	require.NoError(t, srv.CommitFile(t.Context(), "main.tf", []byte("# newer"), "post-tag commit"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	// Ambient depth is full history; only a URL depth can force a shallow clone.
	c, err := cas.New(cas.WithStorePath(storePath), cas.WithCloneDepth(-1))
	require.NoError(t, err)

	v := venv.OSVenv()
	l := logger.CreateLogger()

	g := getter.NewCASGetter(l, c, v, &cas.CloneOptions{})
	client := &getter.Client{Getters: []getter.Getter{g}}

	dst := filepath.Join(tempDir, "dst")
	_, err = client.Get(t.Context(), &getter.Request{
		Src:     "git::" + repoURL + "?depth=1&ref=v1.0.0",
		Dst:     dst,
		GetMode: getter.ModeDir,
	})
	require.NoError(t, err)

	// The tag was checked out correctly (not HEAD)...
	content, err := os.ReadFile(filepath.Join(dst, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# tagged", string(content))

	// ...and the central store's bare repo is shallow, which can only happen
	// if the URL's depth=1 overrode the ambient full-history setting. A
	// strip-and-discard implementation would fetch full history and leave no
	// `shallow` marker.
	bareRepo := singleGitStoreRepo(t, filepath.Join(storePath, "git"))
	assert.FileExists(t, filepath.Join(bareRepo, "shallow"),
		"URL depth=1 must force a shallow fetch even when the ambient clone depth is full history")
}

// singleGitStoreRepo returns the bare-repo path of the one per-URL entry in
// the CAS git store rooted at gitStoreRoot. The test clones a single
// submodule-free URL, so exactly one entry is expected.
func singleGitStoreRepo(t *testing.T, gitStoreRoot string) string {
	t.Helper()

	entries, err := os.ReadDir(gitStoreRoot)
	require.NoError(t, err)

	var dirs []string

	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}

	require.Len(t, dirs, 1, "expected exactly one per-URL bare repo in the git store")

	return filepath.Join(gitStoreRoot, dirs[0], "repo")
}

// resolveHeadE2E is a convenience wrapper used by several tests in
// this file; included here so the file is independent of
// commitref_test.go's helpers.
func resolveHeadE2E(t *testing.T, srv string) string {
	t.Helper()

	results, err := newGitRunner(t).LsRemote(t.Context(), srv, "HEAD")
	require.NoError(t, err)
	require.NotEmpty(t, results)

	return results[0].Hash
}
