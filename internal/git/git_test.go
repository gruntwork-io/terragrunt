package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestGitRunner_LsRemote(t *testing.T) {
	t.Parallel()

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	ctx := t.Context()

	t.Run("valid repository", func(t *testing.T) {
		t.Parallel()

		results, err := runner.LsRemote(
			ctx,
			"https://github.com/gruntwork-io/terragrunt.git",
			"HEAD",
		)
		require.NoError(t, err)
		require.NotEmpty(t, results)
		assert.Regexp(t, "^[0-9a-f]{40}$", results[0].Hash)
		assert.Equal(t, "HEAD", results[0].Ref)
	})

	t.Run("invalid repository", func(t *testing.T) {
		t.Parallel()

		_, err := runner.LsRemote(ctx, "https://github.com/nonexistent/repo.git", "HEAD")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrCommandSpawn)
	})

	t.Run("nonexistent reference", func(t *testing.T) {
		t.Parallel()

		_, err := runner.LsRemote(
			ctx,
			"https://github.com/gruntwork-io/terragrunt.git",
			"nonexistent-branch",
		)
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrNoMatchingReference)
	})
}

func TestGitRunner_Clone(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("shallow clone", func(t *testing.T) {
		t.Parallel()
		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)

		runner = runner.WithWorkDir(cloneDir)
		err = runner.Clone(ctx, "https://github.com/gruntwork-io/terragrunt.git", true, 1, "main")
		require.NoError(t, err)

		// Verify it's a git repository
		_, err = os.Stat(filepath.Join(cloneDir, "HEAD"))
		require.NoError(t, err)
	})

	t.Run("clone without workdir fails", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)
		err = runner.Clone(ctx, "https://github.com/gruntwork-io/terragrunt.git", true, 1, "main")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrNoWorkDir)
	})

	t.Run("invalid repository", func(t *testing.T) {
		t.Parallel()
		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)

		runner = runner.WithWorkDir(cloneDir)
		err = runner.Clone(ctx, "https://github.com/gruntwork-io/terragrunt-fake.git", false, 1, "")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrGitClone)
	})
}

func TestCreateTempDir(t *testing.T) {
	t.Parallel()

	gitRunner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)
	dir, cleanup, err := gitRunner.CreateTempDir()
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, cleanup())
	})

	// Verify directory exists
	_, err = os.Stat(dir)
	require.NoError(t, err)

	// Verify it's empty
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestExtractRepoName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		repo string
		want string
	}{
		{
			name: "simple repo",
			repo: "https://github.com/user/repo.git",
			want: "repo",
		},
		{
			name: "no .git suffix",
			repo: "https://github.com/user/repo",
			want: "repo",
		},
		{
			name: "with path",
			repo: "/path/to/repo.git",
			want: "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, git.ExtractRepoName(tt.repo))
		})
	}
}

func TestGitRunner_LsTree(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("valid repository", func(t *testing.T) {
		t.Parallel()
		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)

		runner = runner.WithWorkDir(cloneDir)

		// First clone a repository
		err = runner.Clone(ctx, "https://github.com/gruntwork-io/terragrunt.git", true, 1, "main")
		require.NoError(t, err)

		// Then try to ls-tree HEAD
		tree, err := runner.LsTreeRecursive(ctx, "HEAD")
		require.NoError(t, err)
		require.NotEmpty(t, tree)
	})

	t.Run("ls-tree without workdir fails", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)

		_, err = runner.LsTreeRecursive(ctx, "HEAD")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrNoWorkDir)
	})

	t.Run("invalid reference", func(t *testing.T) {
		t.Parallel()
		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)

		runner = runner.WithWorkDir(cloneDir)

		// First clone a repository
		err = runner.Clone(ctx, "https://github.com/gruntwork-io/terragrunt.git", true, 1, "main")
		require.NoError(t, err)

		// Try to ls-tree an invalid reference
		_, err = runner.LsTreeRecursive(ctx, "nonexistent")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrReadTree)
	})

	t.Run("invalid repository", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)
		runner = runner.WithWorkDir(helpers.TmpDirWOSymlinks(t))

		// Try to ls-tree in an empty directory
		_, err = runner.LsTreeRecursive(ctx, "HEAD")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrReadTree)
	})
}

func TestGitRunner_RequiresWorkDir(t *testing.T) {
	t.Parallel()

	t.Run("with workdir", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)
		runner = runner.WithWorkDir(helpers.TmpDirWOSymlinks(t))
		err = runner.RequiresWorkDir()
		assert.NoError(t, err)
	})

	t.Run("without workdir", func(t *testing.T) {
		t.Parallel()

		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(t, err)
		err = runner.RequiresWorkDir()
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_InitBare(t *testing.T) {
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

func TestGitRunner_FetchAndHasObject(t *testing.T) {
	t.Parallel()

	srv, err := git.NewServer()
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	require.NoError(t, srv.CommitFile(t.Context(), "README.md", []byte("# test"), "initial"))

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

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

func TestGitRunner_FetchRejectsOptionInjectionRef(t *testing.T) {
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

func TestGitRunner_LsRemoteRejectsOptionInjectionRepo(t *testing.T) {
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

func TestGitRunner_FetchInsertsOptionTerminator(t *testing.T) {
	t.Parallel()

	var got []string

	runner := newArgvCapturingRunner(t, &got, nil).WithWorkDir(helpers.TmpDirWOSymlinks(t))

	require.NoError(t, runner.Fetch(t.Context(), "file:///repo", "somebranch", 1))
	assert.Equal(t,
		[]string{"fetch", "--depth", "1", "--no-tags", "--", "file:///repo", "somebranch"},
		got,
	)
}

func TestGitRunner_CloneInsertsOptionTerminator(t *testing.T) {
	t.Parallel()

	var got []string

	workDir := helpers.TmpDirWOSymlinks(t)
	runner := newArgvCapturingRunner(t, &got, nil).WithWorkDir(workDir)

	require.NoError(t, runner.Clone(t.Context(), "file:///repo", true, 1, "main"))
	assert.Equal(
		t,
		[]string{
			"clone",
			"--bare",
			"--depth",
			"1",
			"--single-branch",
			"--branch",
			"main",
			"--",
			"file:///repo",
			workDir,
		},
		got,
	)
}

func TestGitRunner_LsRemoteInsertsOptionTerminator(t *testing.T) {
	t.Parallel()

	var got []string

	runner := newArgvCapturingRunner(
		t,
		&got,
		[]byte("deadbeefcafefacedeadbeefcafefacedeadbeef\trefs/heads/main\n"),
	)

	_, err := runner.LsRemote(t.Context(), "file:///repo", "main")
	require.NoError(t, err)
	assert.Equal(t, []string{"ls-remote", "--", "file:///repo", "main"}, got)
}

func TestGitRunner_HasObjectSurfacesNonMissingFailures(t *testing.T) {
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

func TestGitRunner_WithWorkDirGetRepoRootWithRacing(t *testing.T) {
	t.Parallel()

	dir := helpers.TmpDirWOSymlinks(t)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(dir)
	require.NoError(t, runner.Init(t.Context()))

	// GetRepoRoot memoizes on first success, so only that first call writes.
	// Derive a fresh runner per round and race the memoizing call against a
	// concurrent WithWorkDir copy of the same runner.
	const rounds = 50

	g, ctx := errgroup.WithContext(t.Context())

	for range rounds {
		fresh := runner.WithWorkDir(dir)

		g.Go(func() error {
			_, err := fresh.GetRepoRoot(ctx)

			return err
		})

		g.Go(func() error {
			return fresh.WithWorkDir(dir).RequiresWorkDir()
		})
	}

	require.NoError(t, g.Wait())
}

// TestGitRunner_AddCommitCheckoutConfig drives the local-mutation wrappers
// (ConfigSet, Add, Commit, Checkout) through a stage -> commit -> branch flow
// against a fresh repository, and checks the state via the read helpers
// (HasUncommittedChanges, GetCurrentBranch, Config) at each step.
func TestGitRunner_AddCommitCheckoutConfig(t *testing.T) {
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

// newArgvCapturingRunner returns a runner backed by an in-memory exec that
// records the argv passed to git into captured and replies with stdout.
func newArgvCapturingRunner(t *testing.T, captured *[]string, stdout []byte) *git.GitRunner {
	t.Helper()

	e := vexec.NewMemExec(
		func(_ context.Context, inv vexec.Invocation) vexec.Result {
			*captured = inv.Args
			return vexec.Result{Stdout: stdout}
		},
		vexec.WithLookPath(func(string) (string, error) { return "git", nil }),
	)

	r, err := git.NewGitRunner(e)
	require.NoError(t, err)

	return r
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
