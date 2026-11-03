package git_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGitRunner(t *testing.T) {
	t.Parallel()

	t.Run("resolved path", func(t *testing.T) {
		t.Parallel()

		e := vexec.NewMemExec(
			staticResult(vexec.Result{}),
			vexec.WithLookPath(func(string) (string, error) {
				return "/usr/bin/git", nil
			}),
		)

		runner, err := git.NewGitRunner(venvtest.New().WithExec(e))
		require.NoError(t, err)
		assert.Equal(t, "/usr/bin/git", runner.GitPath)
	})

	t.Run("missing binary", func(t *testing.T) {
		t.Parallel()

		e := vexec.NewMemExec(
			staticResult(vexec.Result{}),
			vexec.WithLookPath(func(string) (string, error) {
				return "", errors.New("not found")
			}),
		)

		runner, err := git.NewGitRunner(venvtest.New().WithExec(e))
		require.ErrorIs(t, err, git.ErrGitCommand)
		assert.Nil(t, runner)
	})
}

func TestGitRunner_LsRemoteTags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr error
		name    string
		stdout  string
		want    []git.LsRemoteResult
		exit    int
	}{
		{
			name:   "tags",
			stdout: "a\trefs/tags/v1.2.0\nb\trefs/tags/v1.2.0^{}\n",
			want: []git.LsRemoteResult{
				{Hash: "a", Ref: "refs/tags/v1.2.0"},
				{Hash: "b", Ref: "refs/tags/v1.2.0^{}"},
			},
		},
		{
			name:   "no tags",
			stdout: "",
		},
		{
			name:    "command failure",
			exit:    128,
			wantErr: git.ErrGitCommand,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
				assert.Equal(t, []string{"ls-remote", "--", "origin", "refs/tags/*"}, inv.Args)

				return vexec.Result{Stdout: []byte(tc.stdout), ExitCode: tc.exit}
			})

			got, err := runner.LsRemoteTags(t.Context(), "origin")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGitRunner_LocalTags(t *testing.T) {
	t.Parallel()

	t.Run("lists tags", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
			assert.Equal(t, []string{"for-each-ref", "--format=%(objectname) %(refname)", "refs/tags/"}, inv.Args)

			return vexec.Result{Stdout: []byte("a refs/tags/v1.0.0\nb refs/tags/v1.1.0\n")}
		}).WithWorkDir("/repo")

		got, err := runner.LocalTags(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []git.LsRemoteResult{
			{Hash: "a", Ref: "refs/tags/v1.0.0"},
			{Hash: "b", Ref: "refs/tags/v1.1.0"},
		}, got)
	})

	t.Run("skips lines without a hash and a ref", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			Stdout: []byte("incomplete\na refs/tags/v1.0.0\n"),
		})).WithWorkDir("/repo")

		got, err := runner.LocalTags(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []git.LsRemoteResult{{Hash: "a", Ref: "refs/tags/v1.0.0"}}, got)
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 128})).WithWorkDir("/repo")

		_, err := runner.LocalTags(t.Context())
		require.ErrorIs(t, err, git.ErrGitCommand)
	})

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, failIfSpawned(t))

		_, err := runner.LocalTags(t.Context())
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_IsShallow(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr error
		name    string
		stdout  string
		exit    int
		want    bool
	}{
		{name: "shallow", stdout: "true\n", want: true},
		{name: "complete", stdout: "false\n"},
		{name: "command failure", exit: 128, wantErr: git.ErrGitCommand},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
				assert.Equal(t, []string{"rev-parse", "--is-shallow-repository"}, inv.Args)

				return vexec.Result{Stdout: []byte(tc.stdout), ExitCode: tc.exit}
			}).WithWorkDir("/repo")

			got, err := runner.IsShallow(t.Context())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, failIfSpawned(t))

		_, err := runner.IsShallow(t.Context())
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestTreePaths(t *testing.T) {
	t.Parallel()

	tree := git.TreePaths{"a/b.txt": {}, "file": {}}

	testCases := []struct {
		name         string
		path         string
		wantHas      bool
		wantContains bool
	}{
		{name: "top level file", path: "file", wantHas: true, wantContains: true},
		{name: "nested file", path: "a/b.txt", wantHas: true, wantContains: true},
		{name: "directory", path: "a", wantContains: true},
		{name: "prefix that stops mid name", path: "a/b"},
		{name: "absent path", path: "missing"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.wantHas, tree.Has(tc.path))
			assert.Equal(t, tc.wantContains, tree.HasOrContains(tc.path))
		})
	}
}

func TestGitRunner_LsTreeNames(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")

	t.Run("lists from the repository root", func(t *testing.T) {
		t.Parallel()

		fsys := venvtest.NewFS(t, root, map[string]string{".git": "gitdir: /elsewhere\n", "sub/file": ""})

		runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
			assert.Equal(t, []string{"ls-tree", "-r", "--name-only", "-z", "HEAD"}, inv.Args)
			assert.Equal(t, root, inv.Dir, "git limits a listing to the directory it runs in")

			return vexec.Result{Stdout: []byte("a.txt\x00dir/with space.txt\x00")}
		}).WithWorkDir(filepath.Join(root, "sub"))

		tree, err := runner.LsTreeNames(t.Context(), venvtest.New().WithFS(fsys), "HEAD")
		require.NoError(t, err)
		assert.Equal(t, git.TreePaths{"a.txt": {}, "dir/with space.txt": {}}, tree)
	})

	t.Run("empty tree", func(t *testing.T) {
		t.Parallel()

		fsys := venvtest.NewFS(t, root, map[string]string{".git": "gitdir: /elsewhere\n"})
		runner := newMemRunner(t, staticResult(vexec.Result{})).WithWorkDir(root)

		tree, err := runner.LsTreeNames(t.Context(), venvtest.New().WithFS(fsys), "HEAD")
		require.NoError(t, err)
		assert.Empty(t, tree)
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		fsys := venvtest.NewFS(t, root, map[string]string{".git": "gitdir: /elsewhere\n"})
		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 128})).WithWorkDir(root)

		_, err := runner.LsTreeNames(t.Context(), venvtest.New().WithFS(fsys), "HEAD")
		require.ErrorIs(t, err, git.ErrReadTree)
	})

	t.Run("outside a repository", func(t *testing.T) {
		t.Parallel()

		fsys := venvtest.NewFS(t, root, map[string]string{"file": ""})
		runner := newMemRunner(t, failIfSpawned(t)).WithWorkDir(root)

		_, err := runner.LsTreeNames(t.Context(), venvtest.New().WithFS(fsys), "HEAD")
		require.ErrorIs(t, err, git.ErrNoRepoRoot)
	})

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, failIfSpawned(t))

		_, err := runner.LsTreeNames(t.Context(), venvtest.New(), "HEAD")
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_Version(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr error
		name    string
		stdout  string
		want    string
		exit    int
	}{
		{name: "release", stdout: "git version 2.47.1\n", want: "2.47.1"},
		{name: "git for windows suffix", stdout: "git version 2.47.1.windows.2\n", want: "2.47.1"},
		{name: "command failure", exit: 1, wantErr: git.ErrGitCommand},
		{name: "no version field", stdout: "git version\n", wantErr: git.ErrParseVersion},
		{name: "unparsable version", stdout: "git version unknown\n", wantErr: git.ErrParseVersion},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
				assert.Equal(t, []string{"version"}, inv.Args)

				return vexec.Result{Stdout: []byte(tc.stdout), ExitCode: tc.exit}
			})

			got, err := runner.Version(t.Context())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got.String())
		})
	}
}

// TestGitRunner_CancelSignalsTheCommand pins what a cancelled context sends a running git, and which failures count.
func TestGitRunner_CancelSignalsTheCommand(t *testing.T) {
	t.Parallel()

	errSignal := errors.New("signal failed")

	testCases := []struct {
		signalErr error
		cause     error
		wantErr   error
		wantSig   os.Signal
		name      string
	}{
		{
			name:    "plain cancel kills",
			wantSig: os.Kill,
		},
		{
			name:    "cancel by a signal forwards it",
			cause:   signal.NewContextCanceledError(os.Interrupt),
			wantSig: os.Interrupt,
		},
		{
			name:      "a process that never started is not a failure",
			signalErr: vexec.ErrProcessNotStarted,
			wantSig:   os.Kill,
		},
		{
			name:      "failing to signal is reported",
			signalErr: errSignal,
			wantErr:   errSignal,
			wantSig:   os.Kill,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancelCause(t.Context())
			sent := make(chan os.Signal, 1)

			var got os.Signal

			e := &signalRecordingExec{
				Exec: vexec.NewMemExec(func(context.Context, vexec.Invocation) vexec.Result {
					cancel(tc.cause)

					got = <-sent

					return vexec.Result{}
				}),
				sent:      sent,
				signalErr: tc.signalErr,
			}

			runner, err := git.NewGitRunner(venvtest.New().WithExec(e))
			require.NoError(t, err)

			err = runner.WithWorkDir("/repo").ResetHard(ctx, "main")

			assert.Equal(t, tc.wantSig, got)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestReleaseTags(t *testing.T) {
	t.Parallel()

	refs := []git.LsRemoteResult{
		{Hash: "a", Ref: "refs/tags/v1.2.0"},
		{Hash: "b", Ref: "refs/tags/v1.10.0"},
		{Hash: "c", Ref: "refs/tags/v1.10.0^{}"},
		{Hash: "d", Ref: "refs/tags/v2.0.0-rc1"},
		{Hash: "e", Ref: "refs/tags/not-semver"},
		{Hash: "f", Ref: "refs/heads/v9.0.0"},
		{Hash: "g", Ref: "refs/tags/v0.9.0"},
	}

	assert.Equal(t, []git.LsRemoteResult{
		{Hash: "b", Ref: "refs/tags/v1.10.0"},
		{Hash: "a", Ref: "refs/tags/v1.2.0"},
		{Hash: "g", Ref: "refs/tags/v0.9.0"},
	}, git.ReleaseTags(refs))
}

func TestLatestReleaseTag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want string
		refs []git.LsRemoteResult
	}{
		{
			name: "highest stable tag",
			refs: []git.LsRemoteResult{
				{Hash: "a", Ref: "refs/tags/v1.2.0"},
				{Hash: "b", Ref: "refs/tags/v2.0.0-rc1"},
				{Hash: "c", Ref: "refs/tags/not-semver"},
				{Hash: "d", Ref: "refs/tags/v1.10.0"},
				{Hash: "e", Ref: "refs/tags/v1.10.0^{}"},
			},
			want: "v1.10.0",
		},
		{
			name: "no tags",
		},
		{
			name: "no release tags",
			refs: []git.LsRemoteResult{
				{Hash: "a", Ref: "refs/tags/not-semver"},
				{Hash: "b", Ref: "refs/tags/v2.0.0-beta1"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, git.LatestReleaseTag(tc.refs))
		})
	}
}

func TestGitRunner_InitBare(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
			assert.Equal(t, []string{"init", "--bare", "/repo"}, inv.Args)

			return vexec.Result{}
		}).WithWorkDir("/repo")

		require.NoError(t, runner.InitBare(t.Context()))
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 128})).WithWorkDir("/repo")

		err := runner.InitBare(t.Context())
		require.ErrorIs(t, err, git.ErrGitInitBare)
	})

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))

		err := runner.InitBare(t.Context())
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_Fetch(t *testing.T) {
	t.Parallel()

	t.Run("full history", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
			assert.Equal(t, []string{
				"-c", "maintenance.auto=false", "-c", "gc.auto=0",
				"fetch", "--", "file:///repo", "main",
			}, inv.Args)

			return vexec.Result{}
		}).WithWorkDir("/repo")

		require.NoError(t, runner.Fetch(t.Context(), "file:///repo", "main", 0))
	})

	t.Run("depth limited", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
			assert.Equal(t, []string{
				"-c", "maintenance.auto=false", "-c", "gc.auto=0",
				"fetch", "--depth", "1", "--no-tags", "--", "file:///repo", "main",
			}, inv.Args)

			return vexec.Result{}
		}).WithWorkDir("/repo")

		require.NoError(t, runner.Fetch(t.Context(), "file:///repo", "main", 1))
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 128})).WithWorkDir("/repo")

		err := runner.Fetch(t.Context(), "file:///repo", "main", 1)
		require.ErrorIs(t, err, git.ErrGitFetch)
	})

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))

		err := runner.Fetch(t.Context(), "file:///repo", "main", 1)
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_FetchUnshallow(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
			assert.Equal(t, []string{
				"-c", "maintenance.auto=false", "-c", "gc.auto=0",
				"fetch", "--unshallow", "--", "file:///repo", "+refs/*:refs/*",
			}, inv.Args)

			return vexec.Result{}
		}).WithWorkDir("/repo")

		require.NoError(t, runner.FetchUnshallow(t.Context(), "file:///repo", "+refs/*:refs/*"))
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 128})).WithWorkDir("/repo")

		err := runner.FetchUnshallow(t.Context(), "file:///repo", "+refs/*:refs/*")
		require.ErrorIs(t, err, git.ErrGitFetch)
	})

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))

		err := runner.FetchUnshallow(t.Context(), "file:///repo", "+refs/*:refs/*")
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_RevParseCommit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr error
		name    string
		workDir string
		want    string
		result  vexec.Result
	}{
		{
			name:    "commit",
			workDir: "/repo",
			result:  vexec.Result{Stdout: []byte(headHash + "\n")},
			want:    headHash,
		},
		{
			name:    "unknown revision",
			workDir: "/repo",
			result:  vexec.Result{ExitCode: 1},
			wantErr: git.ErrUnknownRevision,
		},
		{
			name:    "spawn failure",
			workDir: "/repo",
			result:  vexec.Result{Err: errors.New("spawn failed")},
			wantErr: git.ErrGitCommand,
		},
		{
			name:    "missing workdir",
			wantErr: git.ErrNoWorkDir,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(tc.result))
			if tc.workDir != "" {
				runner = runner.WithWorkDir(tc.workDir)
			}

			got, err := runner.RevParseCommit(t.Context(), "HEAD")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGitRunner_StartCatFileBatch(t *testing.T) {
	t.Parallel()

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))

		_, err := runner.StartCatFileBatch(t.Context())
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_WorktreeCommands(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		invoke  func(context.Context, *git.GitRunner) error
		wantErr error
		name    string
		args    []string
	}{
		{
			name: "create detached",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.CreateDetachedWorktree(
					ctx,
					venvtest.New(),
					"/worktree",
					"HEAD",
					git.CheckoutFiles,
				)
			},
			args: []string{
				"-c", "checkout.workers=" + strconv.Itoa(vfs.DefaultFSWorkers),
				"worktree", "add", "--detach", "/worktree", "HEAD",
			},
			wantErr: git.ErrGitCommand,
		},
		{
			name: "create detached without checkout",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.CreateDetachedWorktree(
					ctx,
					venvtest.New(),
					"/worktree",
					"HEAD",
					git.SkipCheckout,
				)
			},
			args: []string{
				"-c", "checkout.workers=" + strconv.Itoa(vfs.DefaultFSWorkers),
				"worktree", "add", "--detach", "--no-checkout", "/worktree", "HEAD",
			},
			wantErr: git.ErrGitCommand,
		},
		{
			name: "checkout paths",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.CheckoutPaths(ctx, venvtest.New(), "unit")
			},
			args: []string{
				"-c", "checkout.workers=" + strconv.Itoa(vfs.DefaultFSWorkers),
				"checkout", "HEAD", "--", "unit",
			},
			wantErr: git.ErrGitCommand,
		},
		{
			name: "checkout whole tree",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.CheckoutPaths(ctx, venvtest.New())
			},
			args: []string{
				"-c", "checkout.workers=" + strconv.Itoa(vfs.DefaultFSWorkers),
				"checkout", "--force", "HEAD",
			},
			wantErr: git.ErrGitCommand,
		},
		{
			name: "remove",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.RemoveWorktree(ctx, "/worktree")
			},
			args:    []string{"worktree", "remove", "--force", "/worktree"},
			wantErr: git.ErrGitCommand,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
				assert.Equal(t, tc.args, inv.Args)

				return vexec.Result{}
			}).WithWorkDir("/repo")

			require.NoError(t, tc.invoke(t.Context(), runner))
		})

		t.Run(tc.name+" command failure", func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(
				t,
				staticResult(vexec.Result{ExitCode: 128}),
			).WithWorkDir("/repo")

			err := tc.invoke(t.Context(), runner)
			require.ErrorIs(t, err, tc.wantErr)
		})

		t.Run(tc.name+" missing workdir", func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(vexec.Result{}))

			err := tc.invoke(t.Context(), runner)
			require.ErrorIs(t, err, git.ErrNoWorkDir)
		})
	}
}

func TestGitRunner_Init(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{})).WithWorkDir("/repo")
		require.NoError(t, runner.Init(t.Context()))
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 128})).WithWorkDir("/repo")
		err := runner.Init(t.Context())
		require.ErrorIs(t, err, git.ErrGitCommand)
	})

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))
		err := runner.Init(t.Context())
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_HasUncommittedChanges(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		result vexec.Result
		want   bool
	}{
		{
			name:   "dirty",
			result: vexec.Result{Stdout: []byte(" M file.txt\n")},
			want:   true,
		},
		{
			name:   "clean",
			result: vexec.Result{},
			want:   false,
		},
		{
			name:   "command failure discards stdout",
			result: vexec.Result{Stdout: []byte(" M file.txt\n"), ExitCode: 128},
			want:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spawned := 0
			runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
				spawned++

				assert.Equal(t, []string{"status", "--porcelain"}, inv.Args)
				assert.Equal(t, "/repo", inv.Dir)

				return tc.result
			}).WithWorkDir("/repo")

			assert.Equal(t, tc.want, runner.HasUncommittedChanges(t.Context()))
			assert.Equal(t, 1, spawned)
		})
	}
}

func TestGitRunner_ConfigAndRepositoryState(t *testing.T) {
	t.Parallel()

	t.Run("config", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			Stdout: []byte(" value \n"),
		})).WithWorkDir("/repo")

		got, err := runner.Config(t.Context(), "test.key")
		require.NoError(t, err)
		assert.Equal(t, "value", got)
	})

	t.Run("config command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 1})).WithWorkDir("/repo")

		_, err := runner.Config(t.Context(), "test.key")
		require.ErrorIs(t, err, git.ErrGitCommand)
	})

	t.Run("config missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))

		_, err := runner.Config(t.Context(), "test.key")
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})

	t.Run("remote url", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			Stdout: []byte("https://example.com/repo.git\n"),
		})).WithWorkDir("/repo")

		assert.Equal(t, "https://example.com/repo.git", runner.GetRemoteURL(t.Context()))
	})

	t.Run("remote url command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			Stdout:   []byte("https://example.com/repo.git\n"),
			ExitCode: 1,
		})).WithWorkDir("/repo")

		assert.Empty(t, runner.GetRemoteURL(t.Context()))
	})

	t.Run("remote url missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, failIfSpawned(t))

		assert.Empty(t, runner.GetRemoteURL(t.Context()))
	})

	testCases := []struct {
		read func(context.Context, *git.GitRunner) string
		name string
		args []string
	}{
		{
			name: "current branch",
			read: func(ctx context.Context, runner *git.GitRunner) string {
				return runner.GetCurrentBranch(ctx)
			},
			args: []string{"rev-parse", "--abbrev-ref", "HEAD"},
		},
		{
			name: "head commit",
			read: func(ctx context.Context, runner *git.GitRunner) string {
				return runner.GetHeadCommit(ctx)
			},
			args: []string{"rev-parse", "HEAD"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
				assert.Equal(t, tc.args, inv.Args)

				return vexec.Result{Stdout: []byte("value\n")}
			}).WithWorkDir("/repo")

			assert.Equal(t, "value", tc.read(t.Context(), runner))
		})

		t.Run(tc.name+" command failure", func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(vexec.Result{
				Stdout:   []byte("value\n"),
				ExitCode: 128,
			})).WithWorkDir("/repo")

			assert.Empty(t, tc.read(t.Context(), runner))
		})

		t.Run(tc.name+" missing workdir", func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, failIfSpawned(t))
			assert.Empty(t, tc.read(t.Context(), runner))
		})
	}
}

func TestGitRunner_GetDefaultBranchLocal(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr error
		name    string
		workDir string
		want    string
		result  vexec.Result
	}{
		{
			name:    "remote branch",
			workDir: "/repo",
			result:  vexec.Result{Stdout: []byte("origin/main\n")},
			want:    "main",
		},
		{
			name:    "plain branch",
			workDir: "/repo",
			result:  vexec.Result{Stdout: []byte("main\n")},
			want:    "main",
		},
		{
			name:    "unset remote head",
			workDir: "/repo",
			result:  vexec.Result{Stdout: []byte("origin/HEAD\n")},
			wantErr: git.ErrNoMatchingReference,
		},
		{
			name:    "command failure",
			workDir: "/repo",
			result:  vexec.Result{ExitCode: 128},
			wantErr: git.ErrGitCommand,
		},
		{
			name:    "missing workdir",
			wantErr: git.ErrNoWorkDir,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(tc.result))
			if tc.workDir != "" {
				runner = runner.WithWorkDir(tc.workDir)
			}

			got, err := runner.GetDefaultBranchLocal(t.Context())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGitRunner_GetDefaultBranchRemote(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr error
		name    string
		workDir string
		want    string
		result  vexec.Result
	}{
		{
			name:    "symbolic head",
			workDir: "/repo",
			result: vexec.Result{Stdout: []byte(
				"\nref: refs/heads/main\tHEAD\n" + headHash + "\tHEAD\n",
			)},
			want: "main",
		},
		{
			name:    "unparseable output",
			workDir: "/repo",
			result:  vexec.Result{Stdout: []byte(headHash + "\tHEAD\n")},
			wantErr: git.ErrNoMatchingReference,
		},
		{
			name:    "malformed symbolic head",
			workDir: "/repo",
			result:  vexec.Result{Stdout: []byte("ref:\n")},
			wantErr: git.ErrNoMatchingReference,
		},
		{
			name:    "command failure",
			workDir: "/repo",
			result:  vexec.Result{ExitCode: 128},
			wantErr: git.ErrGitCommand,
		},
		{
			name:    "missing workdir",
			wantErr: git.ErrNoWorkDir,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(tc.result))
			if tc.workDir != "" {
				runner = runner.WithWorkDir(tc.workDir)
			}

			got, err := runner.GetDefaultBranchRemote(t.Context())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestGitRunner_LsRemoteDefaultBranch pins that the lookup asks the remote it
// is given, so it needs no working-directory repository.
func TestGitRunner_LsRemoteDefaultBranch(t *testing.T) {
	t.Parallel()

	const remote = "https://example.com/acme/catalog.git"

	var args []string

	runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
		args = inv.Args

		return vexec.Result{Stdout: []byte("ref: refs/heads/trunk\tHEAD\n" + headHash + "\tHEAD\n")}
	})

	branch, err := runner.LsRemoteDefaultBranch(t.Context(), remote)
	require.NoError(t, err)

	assert.Equal(t, "trunk", branch)
	assert.Equal(t, []string{"ls-remote", "--symref", "--", remote, "HEAD"}, args)
}

func TestGitRunner_GetDefaultBranch(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		want    string
		local   vexec.Result
		remote  vexec.Result
		config  vexec.Result
		setHead vexec.Result
	}{
		{
			name:  "local cache",
			local: vexec.Result{Stdout: []byte("origin/main\n")},
			want:  "main",
		},
		{
			name:   "remote head",
			local:  vexec.Result{Stdout: []byte("origin/HEAD\n")},
			remote: vexec.Result{Stdout: []byte("ref: refs/heads/trunk\tHEAD\n")},
			want:   "trunk",
		},
		{
			name:    "remote head with cache failure",
			local:   vexec.Result{Stdout: []byte("origin/HEAD\n")},
			remote:  vexec.Result{Stdout: []byte("ref: refs/heads/trunk\tHEAD\n")},
			setHead: vexec.Result{ExitCode: 1},
			want:    "trunk",
		},
		{
			name:   "configured fallback",
			local:  vexec.Result{ExitCode: 1},
			remote: vexec.Result{ExitCode: 1},
			config: vexec.Result{Stdout: []byte("develop\n")},
			want:   "develop",
		},
		{
			name:   "main fallback",
			local:  vexec.Result{ExitCode: 1},
			remote: vexec.Result{ExitCode: 1},
			config: vexec.Result{ExitCode: 1},
			want:   "main",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
				switch inv.Args[0] {
				case "rev-parse":
					return tc.local
				case "ls-remote":
					return tc.remote
				case "remote":
					return tc.setHead
				case "config":
					return tc.config
				default:
					assert.Fail(t, "unexpected git command", "args=%v", inv.Args)

					return vexec.Result{ExitCode: 1}
				}
			}).WithWorkDir("/repo")

			got := runner.GetDefaultBranch(t.Context(), logger.CreateLogger())
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGitRunner_SetRemoteHeadAuto(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{})).WithWorkDir("/repo")
		require.NoError(t, runner.SetRemoteHeadAuto(t.Context()))
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 1})).WithWorkDir("/repo")
		err := runner.SetRemoteHeadAuto(t.Context())
		require.ErrorIs(t, err, git.ErrGitCommand)
	})

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))
		err := runner.SetRemoteHeadAuto(t.Context())
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_ObjectFormat(t *testing.T) {
	t.Parallel()

	t.Run("sha256", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			Stdout: []byte("sha256\n"),
		})).WithWorkDir("/repo")

		format, err := runner.ObjectFormat(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "sha256", format)
	})

	t.Run("unsupported command", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 129})).WithWorkDir("/repo")

		format, err := runner.ObjectFormat(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "sha1", format)
	})

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))

		_, err := runner.ObjectFormat(t.Context())
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_MutationCommandFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		invoke func(context.Context, *git.GitRunner) error
		name   string
	}{
		{
			name: "add",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.Add(ctx, "file.txt")
			},
		},
		{
			name: "commit",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.Commit(ctx, "message")
			},
		},
		{
			name: "checkout",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.Checkout(ctx, "main", false)
			},
		},
		{
			name: "config set",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.ConfigSet(ctx, "user.name", "Terragrunt")
			},
		},
		{
			name: "remote add",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.RemoteAdd(ctx, "origin", "https://example.com/repo.git")
			},
		},
		{
			name: "fetch tags",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.FetchTags(ctx, "origin")
			},
		},
		{
			name: "reset hard",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.ResetHard(ctx, "main")
			},
		},
		{
			name: "pull fast forward",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.PullFastForward(ctx, "origin", "main", 0)
			},
		},
		{
			name: "submodule update",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.SubmoduleUpdate(ctx, 0)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 1})).WithWorkDir("/repo")
			err := tc.invoke(t.Context(), runner)
			require.ErrorIs(t, err, git.ErrGitCommand)
		})

		t.Run(tc.name+" missing workdir", func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(vexec.Result{}))
			err := tc.invoke(t.Context(), runner)
			require.ErrorIs(t, err, git.ErrNoWorkDir)
		})
	}
}

// signalRecordingExec hands out commands that report the signal they are sent on sent.
type signalRecordingExec struct {
	vexec.Exec
	signalErr error
	sent      chan<- os.Signal
}

// Command prepares every command through the wrapped Exec.
func (e *signalRecordingExec) Command(ctx context.Context, name string, args ...string) vexec.Cmd {
	return &signalRecordingCmd{Cmd: e.Exec.Command(ctx, name, args...), exec: e}
}

// signalRecordingCmd is a command whose Signal reports to its [signalRecordingExec].
type signalRecordingCmd struct {
	vexec.Cmd
	exec *signalRecordingExec
}

func (c *signalRecordingCmd) Signal(sig os.Signal) error {
	c.exec.sent <- sig

	return c.exec.signalErr
}

// failIfSpawned returns a handler that fails the test if git is ever executed.
func failIfSpawned(t *testing.T) vexec.Handler {
	t.Helper()

	return func(context.Context, vexec.Invocation) vexec.Result {
		assert.Fail(t, "git must not be spawned when no working directory is set")

		return vexec.Result{Stdout: []byte("value\n")}
	}
}
