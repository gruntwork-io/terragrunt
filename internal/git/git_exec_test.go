//go:build exec && !windows

// Real-git parity tests for GitRunner. Everything in this file spawns the
// actual git binary, so it is gated behind the exec tag alongside the vexec
// OS-backend parity tests; the default build pins the same contracts through
// the in-memory exec in git_test.go. All remotes are local ([git.Server] or
// file:// paths), so even these tests stay off the network. Constrained to
// !windows because the unit matrix has only ever run them on ubuntu/macos.

package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecGitRunner_LsRemote(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	url := startCommittedServer(t)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	t.Run("valid repository", func(t *testing.T) {
		t.Parallel()

		results, err := runner.LsRemote(ctx, url, "HEAD")
		require.NoError(t, err)
		require.NotEmpty(t, results)
		assert.Regexp(t, "^[0-9a-f]{40}$", results[0].Hash)
		assert.Equal(t, "HEAD", results[0].Ref)
	})

	t.Run("invalid repository", func(t *testing.T) {
		t.Parallel()

		missing := "file://" + filepath.ToSlash(filepath.Join(t.TempDir(), "missing.git"))

		_, err := runner.LsRemote(ctx, missing, "HEAD")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrCommandSpawn)
	})

	t.Run("nonexistent reference", func(t *testing.T) {
		t.Parallel()

		_, err := runner.LsRemote(ctx, url, "nonexistent-branch")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrNoMatchingReference)
	})
}

func TestExecGitRunner_Clone(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	url := startCommittedServer(t)

	t.Run("shallow clone", func(t *testing.T) {
		t.Parallel()

		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)

		runner = runner.WithWorkDir(cloneDir)
		err = runner.Clone(ctx, url, true, 1, "main")
		require.NoError(t, err)

		// Verify it's a git repository
		_, err = os.Stat(filepath.Join(cloneDir, "HEAD"))
		require.NoError(t, err)
	})

	t.Run("invalid repository", func(t *testing.T) {
		t.Parallel()

		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)

		missing := "file://" + filepath.ToSlash(filepath.Join(t.TempDir(), "missing.git"))

		runner = runner.WithWorkDir(cloneDir)
		err = runner.Clone(ctx, missing, false, 1, "")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrGitClone)
	})
}

func TestExecGitRunner_LsTree(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// LsTreeRecursive only reads, so both subtests can share one clone.
	cloneDir := cloneCommittedServer(t)

	t.Run("valid repository", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)

		tree, err := runner.WithWorkDir(cloneDir).LsTreeRecursive(ctx, "HEAD")
		require.NoError(t, err)
		require.NotEmpty(t, tree.Entries())
	})

	t.Run("invalid reference", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)

		_, err = runner.WithWorkDir(cloneDir).LsTreeRecursive(ctx, "nonexistent")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrReadTree)
	})

	t.Run("invalid repository", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)

		// Try to ls-tree in an empty directory
		_, err = runner.WithWorkDir(helpers.TmpDirWOSymlinks(t)).LsTreeRecursive(ctx, "HEAD")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrReadTree)
	})
}

func TestExecGitRunner_InitBare(t *testing.T) {
	t.Parallel()

	dir := helpers.TmpDirWOSymlinks(t)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(dir)

	require.NoError(t, runner.InitBare(t.Context()))

	_, err = os.Stat(filepath.Join(dir, "HEAD"))
	require.NoError(t, err)

	// Idempotent: second call must not error.
	require.NoError(t, runner.InitBare(t.Context()))
}

func TestExecGitRunner_FetchAndHasObject(t *testing.T) {
	t.Parallel()

	url := startCommittedServer(t)

	dir := helpers.TmpDirWOSymlinks(t)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(dir)
	require.NoError(t, runner.InitBare(t.Context()))

	// HasObject returns false (without error) for a missing object.
	missing := "0000000000000000000000000000000000000000"
	has, err := runner.HasObject(t.Context(), missing)
	require.NoError(t, err)
	assert.False(t, has)

	// Fetch HEAD into the bare repo.
	require.NoError(t, runner.Fetch(t.Context(), url, "HEAD", 0))

	// Resolve the HEAD hash through ls-remote and confirm HasObject is now true.
	results, err := runner.LsRemote(t.Context(), url, "HEAD")
	require.NoError(t, err)
	require.NotEmpty(t, results)

	has, err = runner.HasObject(t.Context(), results[0].Hash)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestExecGitRunner_FetchRejectsOptionInjectionRef(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	src := newCommittedRepo(t)

	bareDir := helpers.TmpDirWOSymlinks(t)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(bareDir)
	require.NoError(t, runner.InitBare(ctx))

	marker := filepath.Join(helpers.TmpDirWOSymlinks(t), "injected")

	// A literal space is fine here since the ref is passed straight to git, not created as a branch name.
	injectedRef := "--upload-pack=touch " + marker

	err = runner.Fetch(ctx, "file://"+src, injectedRef, 1)
	require.Error(t, err)

	var wrappedErr *git.WrappedError
	require.ErrorAs(t, err, &wrappedErr)
	require.ErrorIs(t, wrappedErr.Err, git.ErrGitFetch)

	assert.NoFileExists(t, marker)
}

func TestExecGitRunner_LsRemoteRejectsOptionInjectionRepo(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	marker := filepath.Join(helpers.TmpDirWOSymlinks(t), "injected")

	// A repo beginning with a git option must be treated as a positional.
	injectedRepo := "--upload-pack=touch " + marker

	_, err = runner.LsRemote(ctx, injectedRepo, "HEAD")
	require.Error(t, err)

	assert.NoFileExists(t, marker)
}

func TestExecGitRunner_HasObjectSurfacesNonMissingFailures(t *testing.T) {
	t.Parallel()

	dir := helpers.TmpDirWOSymlinks(t)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(dir)
	require.NoError(t, runner.InitBare(t.Context()))

	// A malformed object name returns exit 128 (fatal). HasObject must
	// return an error rather than report missing, so a corrupted store
	// does not trigger a refetch loop.
	_, err = runner.HasObject(t.Context(), "not-a-hash")
	require.Error(t, err)
}

// TestGitRunner_AddCommitCheckoutConfig drives the local-mutation wrappers
// (ConfigSet, Add, Commit, Checkout) through a stage -> commit -> branch flow
// against a fresh repository, and checks the state via the read helpers
// (HasUncommittedChanges, GetCurrentBranch, Config) at each step.
func TestExecGitRunner_AddCommitCheckoutConfig(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	dir := helpers.TmpDirWOSymlinks(t)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(dir)
	require.NoError(t, runner.Init(ctx))

	require.NoError(t, runner.ConfigSet(ctx, "user.email", "test@example.com"))
	require.NoError(t, runner.ConfigSet(ctx, "user.name", "Terragrunt Test"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0o600))

	require.NoError(t, runner.Add(ctx, "hello.txt"))
	assert.True(t, runner.HasUncommittedChanges(ctx))

	require.NoError(t, runner.Commit(ctx, "initial commit"))
	assert.False(t, runner.HasUncommittedChanges(ctx))

	require.NoError(t, runner.Checkout(ctx, "feature", true))
	assert.Equal(t, "feature", runner.GetCurrentBranch(ctx))

	email, err := runner.Config(ctx, "user.email")
	require.NoError(t, err)
	assert.Equal(t, "test@example.com", email)
}

// TestGitRunner_SubmoduleURLs exercises the `git config --blob` path
// against a real repository: the .gitmodules blob committed by the test
// server is located through ls-tree and parsed by git itself.
func TestExecGitRunner_SubmoduleURLs(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	srv, err := git.NewServer()
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	require.NoError(t, srv.CommitFile(t.Context(), "README.md", []byte("# repo"), "add readme"))

	const pinnedHash = "0123456789abcdef0123456789abcdef01234567"

	require.NoError(t, srv.CommitSubmodule(
		t.Context(), "modules/child", "https://example.com/child.git", pinnedHash, "add submodule",
	))

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(helpers.TmpDirWOSymlinks(t))
	require.NoError(t, runner.Clone(ctx, url, true, 0, ""))

	tree, err := runner.LsTreeRecursive(ctx, "HEAD")
	require.NoError(t, err)

	var gitmodulesHash string

	for _, entry := range tree.Entries() {
		if entry.Path == git.GitmodulesPath {
			gitmodulesHash = entry.Hash
		}
	}

	require.NotEmpty(t, gitmodulesHash, ".gitmodules entry not found in tree")

	urls, err := runner.SubmoduleURLs(ctx, gitmodulesHash)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"modules/child": "https://example.com/child.git"}, urls)
}

// startCommittedServer starts a local git HTTP server seeded with a single
// commit and returns its clone URL.
func startCommittedServer(t *testing.T) string {
	t.Helper()

	srv, err := git.NewServer()
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	require.NoError(t, srv.CommitFile(t.Context(), "README.md", []byte("# test"), "initial"))

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	return url
}

// cloneCommittedServer clones a freshly seeded local server into a bare repo
// and returns the clone directory.
func cloneCommittedServer(t *testing.T) string {
	t.Helper()

	url := startCommittedServer(t)

	cloneDir := helpers.TmpDirWOSymlinks(t)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	require.NoError(t, runner.WithWorkDir(cloneDir).Clone(t.Context(), url, true, 1, "main"))

	return cloneDir
}

// newCommittedRepo creates a local repo with one commit for use as a file:// remote.
func newCommittedRepo(t *testing.T) string {
	t.Helper()

	ctx := t.Context()

	dir := helpers.TmpDirWOSymlinks(t)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(dir)

	require.NoError(t, runner.Init(ctx))
	require.NoError(t, runner.ConfigSet(ctx, "user.email", "test@example.com"))
	require.NoError(t, runner.ConfigSet(ctx, "user.name", "Terragrunt Test"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(""), 0o600))
	require.NoError(t, runner.Add(ctx, "main.tf"))
	require.NoError(t, runner.Commit(ctx, "initial commit"))

	return dir
}
