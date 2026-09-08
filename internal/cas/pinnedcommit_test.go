package cas_test

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// deepHistoryCommits and pinnedCommitOffset shape the fixture behind the
// pinned-SHA tests: a history long enough that fetching one commit and
// fetching every commit are plainly different transfers, with the pinned
// commit well behind the tip of main.
const (
	deepHistoryCommits = 200
	pinnedCommitOffset = 50
)

// TestGitStoreEnsureCommit_PinnedSHAFetchesOneCommit pins the cheap path for
// a commit the caller named by full object name: the bare repository ends up
// shallow, which only a depth-limited fetch produces, and the tree behind the
// commit is still readable.
func TestGitStoreEnsureCommit_PinnedSHAFetchesOneCommit(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	srv := newEmptyTestServer(t)

	require.NoError(t, srv.CommitFile(ctx, "README.md", []byte("# deep"), "root"))
	require.NoError(t, srv.CommitEmptyChain(ctx, deepHistoryCommits, "filler"))

	pinned, err := srv.Head(ctx)
	require.NoError(t, err)

	require.NoError(t, srv.CommitEmptyChain(ctx, pinnedCommitOffset, "filler"))

	tip, err := srv.Head(ctx)
	require.NoError(t, err)
	require.NotEqual(t, pinned, tip)

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	store, v, _ := newTestGitStore(t)
	l := logger.CreateLogger()

	repo, err := store.EnsureCommit(ctx, l, v, url, pinned, "")
	require.NoError(t, err)
	assert.Equal(t, pinned, repo.Hash)

	_, err = v.FS.Stat(filepath.Join(repo.Path, "shallow"))
	require.NoError(t, err, "a pinned SHA must arrive through a depth-limited fetch")

	runner, err := git.NewGitRunner(v)
	require.NoError(t, err)

	tree, err := runner.WithWorkDir(repo.Path).LsTreeRecursive(ctx, pinned)
	require.NoError(t, err, "ls-tree must read a commit at the shallow boundary")
	require.Len(t, tree.Entries(), 1)
	assert.Equal(t, "README.md", tree.Entries()[0].Path)

	require.NoError(t, repo.Unlock())

	// A branch fetch into the same bare repository grafts onto the recorded
	// shallow boundary instead of unshallowing it. That serves the store,
	// which reads named commits with ls-tree and cat-file and never walks
	// history, so both the tip and the pinned commit stay readable.
	branchRepo, err := store.EnsureRef(ctx, l, v, url, "main", tip, 0)
	require.NoError(t, err)

	tipTree, err := runner.WithWorkDir(branchRepo.Path).LsTreeRecursive(ctx, tip)
	require.NoError(t, err)
	require.Len(t, tipTree.Entries(), 1)

	stillThere, err := runner.WithWorkDir(branchRepo.Path).HasObject(ctx, pinned)
	require.NoError(t, err)
	assert.True(t, stillThere, "the branch fetch must not cost us the pinned commit")

	require.NoError(t, branchRepo.Unlock())
}

// TestGitStoreEnsureCommit_PinnedSHADeepensForLaterRefs pins what the
// pinned-SHA fetch must not cost the next source naming the same URL. The
// per-URL repository is shared, and a ref only the history behind the
// shallow boundary can resolve, such as an abbreviated SHA of an older
// commit, has to resolve there anyway.
func TestGitStoreEnsureCommit_PinnedSHADeepensForLaterRefs(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	srv := newEmptyTestServer(t)

	require.NoError(t, srv.CommitFile(ctx, "README.md", []byte("# deep"), "root"))

	older, err := srv.Head(ctx)
	require.NoError(t, err)

	require.NoError(t, srv.CommitEmptyChain(ctx, pinnedCommitOffset, "filler"))

	pinned, err := srv.Head(ctx)
	require.NoError(t, err)

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	store, v, _ := newTestGitStore(t)
	l := logger.CreateLogger()

	repo, err := store.EnsureCommit(ctx, l, v, url, pinned, "")
	require.NoError(t, err)

	_, err = v.FS.Stat(filepath.Join(repo.Path, "shallow"))
	require.NoError(t, err, "a pinned SHA must arrive through a depth-limited fetch")

	require.NoError(t, repo.Unlock())

	const abbrevLen = 12

	deepened, err := store.EnsureCommit(ctx, l, v, url, older[:abbrevLen], "")
	require.NoError(t, err, "a later ref must reach the history behind the boundary")
	assert.Equal(t, older, deepened.Hash)

	require.NoError(t, deepened.Unlock())
}

// TestGitStoreEnsureCommit_ServerRefusingObjectNameFallsBack covers remotes
// that will not answer a want line naming an object they never advertised.
// Protocol v2 answers one unconditionally, so the refusal reproduces only
// against a client on the older protocol, which is what a pre-v2 server
// negotiates.
func TestGitStoreEnsureCommit_ServerRefusingObjectNameFallsBack(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	srv := newEmptyTestServer(t)

	require.NoError(t, srv.CommitFile(ctx, "README.md", []byte("# refused"), "root"))
	require.NoError(t, srv.CommitEmptyChain(ctx, pinnedCommitOffset, "filler"))

	pinned, err := srv.Head(ctx)
	require.NoError(t, err)

	require.NoError(t, srv.CommitEmptyChain(ctx, pinnedCommitOffset, "filler"))

	refuseUnadvertisedWants(t, srv)

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	v := oldProtocolVenv()
	store := cas.NewGitStore(filepath.Join(helpers.TmpDirWOSymlinks(t), "gitstore"))

	repo, err := store.EnsureCommit(ctx, logger.CreateLogger(), v, url, pinned, "")
	require.NoError(t, err, "a refused object name must fall back to the full fetch")
	assert.Equal(t, pinned, repo.Hash)

	_, err = v.FS.Stat(filepath.Join(repo.Path, "shallow"))
	require.ErrorIs(t, err, fs.ErrNotExist, "the fallback fetch carries full history")

	require.NoError(t, repo.Unlock())
}

// TestGitStoreEnsureCommit_ServerRefusingObjectNameKeepsKnownHashPath covers
// the same refusal reached through the knownHash argument, the path a peer's
// git-gc leaves behind after [cas.GitStore.ProbeCachedCommit] has already
// canonicalized the ref.
func TestGitStoreEnsureCommit_ServerRefusingObjectNameKeepsKnownHashPath(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	srv := newEmptyTestServer(t)

	require.NoError(t, srv.CommitFile(ctx, "README.md", []byte("# known"), "root"))
	require.NoError(t, srv.CommitEmptyChain(ctx, pinnedCommitOffset, "filler"))

	pinned, err := srv.Head(ctx)
	require.NoError(t, err)

	// The refusal only bites on an object no ref advertises, so the pinned
	// commit has to fall behind the tip of main.
	require.NoError(t, srv.CommitEmptyChain(ctx, pinnedCommitOffset, "filler"))

	refuseUnadvertisedWants(t, srv)

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	v := oldProtocolVenv()
	store := cas.NewGitStore(filepath.Join(helpers.TmpDirWOSymlinks(t), "gitstore"))

	repo, err := store.EnsureCommit(ctx, logger.CreateLogger(), v, url, pinned, pinned)
	require.NoError(t, err)
	assert.Equal(t, pinned, repo.Hash)

	_, err = v.FS.Stat(filepath.Join(repo.Path, "shallow"))
	require.ErrorIs(t, err, fs.ErrNotExist, "the fallback fetch carries full history")

	require.NoError(t, repo.Unlock())
}

// refuseUnadvertisedWants turns off every upload-pack policy that would let
// srv answer a want line naming an object it did not advertise.
func refuseUnadvertisedWants(t *testing.T, srv *git.Server) {
	t.Helper()

	for _, knob := range []string{
		"uploadpack.allowAnySHA1InWant",
		"uploadpack.allowReachableSHA1InWant",
		"uploadpack.allowTipSHA1InWant",
	} {
		require.NoError(t, srv.SetConfig(t.Context(), knob, "false"))
	}
}

// oldProtocolVenv returns an OS-backed environment that pins git to the
// pre-v2 wire protocol. GIT_CONFIG_COUNT is the only way in: the runner
// starts git with the environment the venv carries and nothing else, so a
// config file would never be read.
func oldProtocolVenv() *venv.Venv {
	return venv.OSVenv().WithEnv(map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "protocol.version",
		"GIT_CONFIG_VALUE_0": "0",
	})
}
