package git_test

import (
	"context"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

const headHash = "deadbeefcafefacedeadbeefcafefacedeadbeef"

func TestGitRunner_LsRemote(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("parses hash and ref", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			Stdout: []byte(headHash + "\tHEAD\n"),
		}))

		results, err := runner.LsRemote(ctx, "https://example.com/repo.git", "HEAD")
		require.NoError(t, err)
		require.NotEmpty(t, results)
		assert.Equal(t, headHash, results[0].Hash)
		assert.Equal(t, "HEAD", results[0].Ref)
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			ExitCode: 128,
			Stderr:   []byte("fatal: repository not found"),
		}))

		_, err := runner.LsRemote(ctx, "https://example.com/nonexistent.git", "HEAD")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrCommandSpawn)
	})

	t.Run("nonexistent reference", func(t *testing.T) {
		t.Parallel()

		// ls-remote exits zero with empty output when the ref does not exist.
		runner := newMemRunner(t, staticResult(vexec.Result{}))

		_, err := runner.LsRemote(ctx, "https://example.com/repo.git", "nonexistent-branch")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrNoMatchingReference)
	})
}

func TestGitRunner_Clone(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("clone without workdir fails", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, func(context.Context, vexec.Invocation) vexec.Result {
			t.Error("git must not be spawned when no working directory is set")

			return vexec.Result{}
		})

		err := runner.Clone(ctx, "https://example.com/repo.git", true, 1, "main")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrNoWorkDir)
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			ExitCode: 128,
			Stderr:   []byte("fatal: repository not found"),
		}))

		err := runner.WithWorkDir(t.TempDir()).Clone(ctx, "https://example.com/nonexistent.git", false, 1, "")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrGitClone)
	})
}

func TestGitRunner_LsTree(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("parses recursive tree", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			Stdout: []byte("100644 blob aaaabeefcafefacedeadbeefcafefacedeadbeef\tREADME.md\n" +
				"100644 blob bbbbbeefcafefacedeadbeefcafefacedeadbeef\tdir/file.txt\n"),
		}))

		tree, err := runner.WithWorkDir(t.TempDir()).LsTreeRecursive(ctx, "HEAD")
		require.NoError(t, err)

		entries := tree.Entries()
		require.Len(t, entries, 2)
		assert.Equal(t, "README.md", entries[0].Path)
		assert.Equal(t, "dir/file.txt", entries[1].Path)
	})

	t.Run("ls-tree without workdir fails", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))

		_, err := runner.LsTreeRecursive(ctx, "HEAD")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrNoWorkDir)
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			ExitCode: 128,
			Stderr:   []byte("fatal: not a tree object"),
		}))

		_, err := runner.WithWorkDir(t.TempDir()).LsTreeRecursive(ctx, "nonexistent")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrReadTree)
	})
}

func TestGitRunner_HasObject(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("present object", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))

		has, err := runner.WithWorkDir(t.TempDir()).HasObject(ctx, headHash)
		require.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("missing object", func(t *testing.T) {
		t.Parallel()

		// `git cat-file -e` exits 1 for an absent object.
		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 1}))

		has, err := runner.WithWorkDir(t.TempDir()).HasObject(ctx, headHash)
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("fatal failure surfaces as error", func(t *testing.T) {
		t.Parallel()

		// Exit 128 (e.g. corrupted store) must be an error rather than a
		// missing-object report, so callers do not loop into a refetch.
		runner := newMemRunner(t, staticResult(vexec.Result{
			ExitCode: 128,
			Stderr:   []byte("fatal: not a valid object name"),
		}))

		_, err := runner.WithWorkDir(t.TempDir()).HasObject(ctx, "not-a-hash")
		require.Error(t, err)
	})
}

func TestCreateTempDir(t *testing.T) {
	t.Parallel()

	gitRunner := newMemRunner(t, staticResult(vexec.Result{}))

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

func TestGitRunner_RequiresWorkDir(t *testing.T) {
	t.Parallel()

	t.Run("with workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))
		err := runner.WithWorkDir(t.TempDir()).RequiresWorkDir()
		assert.NoError(t, err)
	})

	t.Run("without workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))
		err := runner.RequiresWorkDir()
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
		assert.ErrorIs(t, wrappedErr.Err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_FetchInsertsOptionTerminator(t *testing.T) {
	t.Parallel()

	var got []string

	runner := newArgvCapturingRunner(t, &got, nil).WithWorkDir(t.TempDir())

	require.NoError(t, runner.Fetch(t.Context(), "file:///repo", "somebranch", 1))
	assert.Equal(t,
		[]string{"fetch", "--depth", "1", "--no-tags", "--", "file:///repo", "somebranch"},
		got,
	)
}

func TestGitRunner_CloneInsertsOptionTerminator(t *testing.T) {
	t.Parallel()

	var got []string

	workDir := t.TempDir()
	runner := newArgvCapturingRunner(t, &got, nil).WithWorkDir(workDir)

	require.NoError(t, runner.Clone(t.Context(), "file:///repo", true, 1, "main"))
	assert.Equal(t,
		[]string{"clone", "--bare", "--depth", "1", "--single-branch", "--branch", "main", "--", "file:///repo", workDir},
		got,
	)
}

func TestGitRunner_LsRemoteInsertsOptionTerminator(t *testing.T) {
	t.Parallel()

	var got []string

	runner := newArgvCapturingRunner(t, &got, []byte(headHash+"\trefs/heads/main\n"))

	_, err := runner.LsRemote(t.Context(), "file:///repo", "main")
	require.NoError(t, err)
	assert.Equal(t, []string{"ls-remote", "--", "file:///repo", "main"}, got)
}

// TestGitRunner_ArgvConstruction pins the argv each wrapper hands to git.
// Commands taking remote URLs are pinned separately by the
// *InsertsOptionTerminator tests; behavior tests above stay argv-agnostic.
func TestGitRunner_ArgvConstruction(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	tests := []struct {
		invoke func(ctx context.Context, r *git.GitRunner) error
		name   string
		want   []string
	}{
		{
			name: "ls-tree recursive",
			invoke: func(ctx context.Context, r *git.GitRunner) error {
				_, err := r.LsTreeRecursive(ctx, "HEAD")
				return err
			},
			want: []string{"ls-tree", "-r", "HEAD"},
		},
		{
			name: "cat-file existence check",
			invoke: func(ctx context.Context, r *git.GitRunner) error {
				_, err := r.HasObject(ctx, headHash)
				return err
			},
			want: []string{"cat-file", "-e", headHash},
		},
		{
			name: "repo root",
			invoke: func(ctx context.Context, r *git.GitRunner) error {
				_, err := r.GetRepoRoot(ctx)
				return err
			},
			want: []string{"rev-parse", "--show-toplevel"},
		},
		{
			name: "add",
			invoke: func(ctx context.Context, r *git.GitRunner) error {
				return r.Add(ctx, "a.txt", "b.txt")
			},
			want: []string{"add", "a.txt", "b.txt"},
		},
		{
			name: "commit splices flags before message",
			invoke: func(ctx context.Context, r *git.GitRunner) error {
				return r.Commit(ctx, "initial commit", "--amend")
			},
			want: []string{"commit", "--amend", "-m", "initial commit"},
		},
		{
			name: "checkout new branch",
			invoke: func(ctx context.Context, r *git.GitRunner) error {
				return r.Checkout(ctx, "feature", true)
			},
			want: []string{"checkout", "-b", "feature"},
		},
		{
			name: "checkout existing branch",
			invoke: func(ctx context.Context, r *git.GitRunner) error {
				return r.Checkout(ctx, "main", false)
			},
			want: []string{"checkout", "main"},
		},
		{
			name: "config set",
			invoke: func(ctx context.Context, r *git.GitRunner) error {
				return r.ConfigSet(ctx, "user.email", "test@example.com")
			},
			want: []string{"config", "user.email", "test@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got []string

			runner := newArgvCapturingRunner(t, &got, nil).WithWorkDir(workDir)

			require.NoError(t, tt.invoke(t.Context(), runner))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGitRunner_WithWorkDirGetRepoRootWithRacing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	runner := newMemRunner(t, staticResult(vexec.Result{Stdout: []byte(dir + "\n")}))
	runner = runner.WithWorkDir(dir)

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

// newMemRunner returns a runner backed by an in-memory exec dispatching every
// git invocation to h.
func newMemRunner(t *testing.T, h vexec.Handler) *git.GitRunner {
	t.Helper()

	r, err := git.NewGitRunner(vexec.NewMemExec(h))
	require.NoError(t, err)

	return r
}

// staticResult returns a handler replying with the same result to every
// invocation.
func staticResult(res vexec.Result) vexec.Handler {
	return func(context.Context, vexec.Invocation) vexec.Result {
		return res
	}
}

// newArgvCapturingRunner returns a runner backed by an in-memory exec that
// records the argv passed to git into captured and replies with stdout.
func newArgvCapturingRunner(t *testing.T, captured *[]string, stdout []byte) *git.GitRunner {
	t.Helper()

	e := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		*captured = inv.Args
		return vexec.Result{Stdout: stdout}
	})

	r, err := git.NewGitRunner(e)
	require.NoError(t, err)

	return r
}
