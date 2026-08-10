package git_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
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

		runner, err := git.NewGitRunner(e)
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

		runner, err := git.NewGitRunner(e)
		require.ErrorIs(t, err, git.ErrCommandSpawn)
		assert.Nil(t, runner)
	})
}

func TestGitRunner_WithWorkDir(t *testing.T) {
	t.Parallel()

	t.Run("nil receiver", func(t *testing.T) {
		t.Parallel()

		var runner *git.GitRunner

		got := runner.WithWorkDir("/repo")
		assert.Equal(t, "/repo", got.WorkDir)
	})
}

func TestGitRunner_GetRepoRoot(t *testing.T) {
	t.Parallel()

	t.Run("memoizes success", func(t *testing.T) {
		t.Parallel()

		calls := 0
		runner := newMemRunner(t, func(context.Context, vexec.Invocation) vexec.Result {
			calls++

			return vexec.Result{Stdout: []byte("/repo\n")}
		}).WithWorkDir("/repo/unit")

		for range 2 {
			root, err := runner.GetRepoRoot(t.Context())
			require.NoError(t, err)
			assert.Equal(t, "/repo", root)
		}

		assert.Equal(t, 1, calls)
	})

	t.Run("retries failure", func(t *testing.T) {
		t.Parallel()

		calls := 0
		runner := newMemRunner(t, func(context.Context, vexec.Invocation) vexec.Result {
			calls++
			if calls == 1 {
				return vexec.Result{ExitCode: 128}
			}

			return vexec.Result{Stdout: []byte("/repo\n")}
		}).WithWorkDir("/repo/unit")

		_, err := runner.GetRepoRoot(t.Context())
		require.ErrorIs(t, err, git.ErrCommandSpawn)

		root, err := runner.GetRepoRoot(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "/repo", root)
		assert.Equal(t, 2, calls)
	})

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))

		_, err := runner.GetRepoRoot(t.Context())
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_LatestReleaseTag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr error
		name    string
		stdout  string
		want    string
		exit    int
	}{
		{
			name: "highest stable tag",
			stdout: "a\trefs/tags/v1.2.0\n" +
				"b\trefs/tags/v2.0.0-rc1\n" +
				"c\trefs/tags/not-semver\n" +
				"d\trefs/tags/v1.10.0\n" +
				"e\trefs/tags/v1.10.0^{}\n",
			want: "v1.10.0",
		},
		{
			name:   "no tags",
			stdout: "",
			want:   "",
		},
		{
			name:   "no release tags",
			stdout: "a\trefs/tags/not-semver\nb\trefs/tags/v2.0.0-beta1\n",
			want:   "",
		},
		{
			name:    "command failure",
			exit:    128,
			wantErr: git.ErrCommandSpawn,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(vexec.Result{
				Stdout:   []byte(tc.stdout),
				ExitCode: tc.exit,
			}))

			got, err := runner.LatestReleaseTag(t.Context(), "origin")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
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
			assert.Equal(t, []string{"fetch", "--", "file:///repo", "main"}, inv.Args)

			return vexec.Result{}
		}).WithWorkDir("/repo")

		require.NoError(t, runner.Fetch(t.Context(), "file:///repo", "main", 0))
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
			wantErr: git.ErrCommandSpawn,
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

func TestGitRunner_CatFile(t *testing.T) {
	t.Parallel()

	t.Run("writes object", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			Stdout: []byte("content"),
		})).WithWorkDir("/repo")

		var output bytes.Buffer

		require.NoError(t, runner.CatFile(t.Context(), headHash, &output))
		assert.Equal(t, "content", output.String())
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 128})).WithWorkDir("/repo")

		err := runner.CatFile(t.Context(), headHash, &bytes.Buffer{})
		require.ErrorIs(t, err, git.ErrCommandSpawn)
	})

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))

		err := runner.CatFile(t.Context(), headHash, &bytes.Buffer{})
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}

func TestGitRunner_WorktreeCommands(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		invoke func(context.Context, *git.GitRunner) error
		name   string
		args   []string
	}{
		{
			name: "create detached",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.CreateDetachedWorktree(ctx, "/worktree", "HEAD")
			},
			args: []string{"worktree", "add", "--detach", "/worktree", "HEAD"},
		},
		{
			name: "remove",
			invoke: func(ctx context.Context, runner *git.GitRunner) error {
				return runner.RemoveWorktree(ctx, "/worktree")
			},
			args: []string{"worktree", "remove", "--force", "/worktree"},
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

			runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 128})).WithWorkDir("/repo")

			err := tc.invoke(t.Context(), runner)
			require.ErrorIs(t, err, git.ErrCommandSpawn)
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
		require.ErrorIs(t, err, git.ErrCommandSpawn)
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
			name: "clean",
		},
		{
			name:   "command failure",
			result: vexec.Result{ExitCode: 128},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(tc.result)).WithWorkDir("/repo")
			assert.Equal(t, tc.want, runner.HasUncommittedChanges(t.Context()))
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
		require.ErrorIs(t, err, git.ErrCommandSpawn)
		assert.Empty(t, runner.GetRemoteURL(t.Context()))
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

			runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 128})).WithWorkDir("/repo")
			assert.Empty(t, tc.read(t.Context(), runner))
		})

		t.Run(tc.name+" missing workdir", func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(vexec.Result{}))
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
			wantErr: git.ErrCommandSpawn,
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
			wantErr: git.ErrCommandSpawn,
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
			name:    "remote head",
			local:   vexec.Result{Stdout: []byte("origin/HEAD\n")},
			remote:  vexec.Result{Stdout: []byte("ref: refs/heads/trunk\tHEAD\n")},
			setHead: vexec.Result{},
			want:    "trunk",
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
					t.Errorf("unexpected git command: %v", inv.Args)

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
		require.ErrorIs(t, err, git.ErrCommandSpawn)
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 1})).WithWorkDir("/repo")
			err := tc.invoke(t.Context(), runner)
			require.ErrorIs(t, err, git.ErrCommandSpawn)
		})

		t.Run(tc.name+" missing workdir", func(t *testing.T) {
			t.Parallel()

			runner := newMemRunner(t, staticResult(vexec.Result{}))
			err := tc.invoke(t.Context(), runner)
			require.ErrorIs(t, err, git.ErrNoWorkDir)
		})
	}
}
