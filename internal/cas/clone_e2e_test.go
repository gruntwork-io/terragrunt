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
