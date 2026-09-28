package discovery_test

import (
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/shell/split"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewForDiscoveryCommand_QueueConstructAs(t *testing.T) {
	t.Parallel()

	newForDiscoveryCommand := func(t *testing.T, queueConstructAs string) (*discovery.Discovery, error) {
		t.Helper()

		v := venvtest.New()

		return discovery.NewForDiscoveryCommand(
			logger.CreateLogger(),
			v.FS,
			&discovery.DiscoveryCommandOptions{
				WorkingDir:       venvtest.Root("/repo"),
				QueueConstructAs: queueConstructAs,
			},
		)
	}

	emptyCases := []struct {
		name             string
		queueConstructAs string
	}{
		{name: "whitespace", queueConstructAs: "   "},
		{name: "tab", queueConstructAs: "\t"},
		{name: "double quoted empty string", queueConstructAs: `""`},
		{name: "single quoted empty string", queueConstructAs: "''"},
	}

	for _, tc := range emptyCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := newForDiscoveryCommand(t, tc.queueConstructAs)
			require.ErrorAs(t, err, &discovery.EmptyQueueConstructAsError{})
		})
	}

	operatorCases := []struct {
		name             string
		queueConstructAs string
	}{
		{name: "semicolon", queueConstructAs: ";"},
		{name: "pipe", queueConstructAs: "|"},
		{name: "logical and", queueConstructAs: "&&"},
		{name: "redirect", queueConstructAs: ">"},
		{name: "stderr redirect", queueConstructAs: "2>&1"},
		{name: "command after separator", queueConstructAs: "; plan"},
		{name: "separator after command", queueConstructAs: "plan; apply"},
	}

	for _, tc := range operatorCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := newForDiscoveryCommand(t, tc.queueConstructAs)
			require.ErrorIs(t, err, split.ErrShellOperator)
		})
	}

	t.Run("command with arguments", func(t *testing.T) {
		t.Parallel()

		d, err := newForDiscoveryCommand(t, "apply -destroy")
		require.NoError(t, err)
		require.NotNil(t, d)
	})

	t.Run("unbalanced quote", func(t *testing.T) {
		t.Parallel()

		_, err := newForDiscoveryCommand(t, "plan '")
		require.Error(t, err)
		require.NotErrorAs(t, err, &discovery.EmptyQueueConstructAsError{})
	})
}

func TestNewForStackGenerate_DiscoveryBoundary(t *testing.T) {
	t.Parallel()

	repoRoot := venvtest.Root("/repo")
	liveDir := filepath.Join(repoRoot, "live")

	v := memRepoRootVenv(t, repoRoot)

	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(liveDir, ".keep"), nil, 0o644))

	t.Run("valid boundary returns discovery", func(t *testing.T) {
		t.Parallel()

		d, err := discovery.NewForStackGenerate(
			logger.CreateLogger(),
			v.FS,
			discovery.StackGenerateOptions{
				WorkingDir:        liveDir,
				DiscoveryBoundary: liveDir,
			},
		)
		require.NoError(t, err)
		require.NotNil(t, d)
	})

	t.Run("nonexistent boundary is rejected", func(t *testing.T) {
		t.Parallel()

		_, err := discovery.NewForStackGenerate(
			logger.CreateLogger(),
			v.FS,
			discovery.StackGenerateOptions{
				WorkingDir:        liveDir,
				DiscoveryBoundary: filepath.Join(repoRoot, "does-not-exist"),
			},
		)
		require.ErrorAs(t, err, &discovery.DiscoveryBoundaryDirError{})
	})

	t.Run("empty boundary is a no-op", func(t *testing.T) {
		t.Parallel()

		d, err := discovery.NewForStackGenerate(
			logger.CreateLogger(),
			v.FS,
			discovery.StackGenerateOptions{
				WorkingDir:        liveDir,
				DiscoveryBoundary: "",
			},
		)
		require.NoError(t, err)
		require.NotNil(t, d)
	})
}

func TestWorktreeBoundary(t *testing.T) {
	t.Parallel()

	repoRoot := venvtest.Root("/repo")
	liveDir := filepath.Join(repoRoot, "live")
	stagingDir := filepath.Join(liveDir, "staging")
	siblingDir := filepath.Join(liveDir, "production")

	v := memRepoRootVenv(t, repoRoot)

	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(stagingDir, ".keep"), nil, 0o644))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(siblingDir, ".keep"), nil, 0o644))

	parseFilters := func(queries ...string) filter.Filters {
		filters, err := filter.ParseFilterQueries(logger.CreateLogger(), queries)
		require.NoError(t, err)

		return filters
	}

	testCases := []struct {
		name     string
		expected string
		v        *venv.Venv
		opts     discovery.StackGenerateOptions
	}{
		{
			name:     "no boundary",
			v:        v,
			opts:     discovery.StackGenerateOptions{WorkingDir: repoRoot},
			expected: "",
		},
		{
			name:     "flag boundary under the git root",
			v:        v,
			opts:     discovery.StackGenerateOptions{WorkingDir: repoRoot, DiscoveryBoundary: liveDir},
			expected: "live",
		},
		{
			name: "Git boundary resolves against the Git root, not the working directory",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir: liveDir,
				Filters:    parseFilters("(./live/staging)...[main...HEAD]"),
			},
			expected: filepath.Join("live", "staging"),
		},
		{
			name: "Git boundary missing from the working tree is kept for the worktrees",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir: repoRoot,
				Filters:    parseFilters("(./live/new)...[main...HEAD]"),
			},
			expected: filepath.Join("live", "new"),
		},
		{
			name:     "boundary equal to the git root",
			v:        v,
			opts:     discovery.StackGenerateOptions{WorkingDir: repoRoot, DiscoveryBoundary: repoRoot},
			expected: "",
		},
		{
			name:     "boundary wider than the working directory keeps its scope",
			v:        v,
			opts:     discovery.StackGenerateOptions{WorkingDir: stagingDir, DiscoveryBoundary: liveDir},
			expected: "live",
		},
		{
			name:     "git root boundary from a nested working directory is unbounded",
			v:        v,
			opts:     discovery.StackGenerateOptions{WorkingDir: stagingDir, DiscoveryBoundary: repoRoot},
			expected: "",
		},
		{
			name:     "Git boundary wider than the working directory keeps its scope",
			v:        v,
			opts:     discovery.StackGenerateOptions{WorkingDir: stagingDir, Filters: parseFilters("(./live)...[main...HEAD]")},
			expected: "live",
		},
		{
			name: "an unbounded filter",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir: repoRoot,
				Filters:    parseFilters("(./live)...[main...HEAD]", "[main...HEAD]"),
			},
			expected: "",
		},
		{
			name: "flag is relative to the Git root for a dependents filter",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir:        repoRoot,
				DiscoveryBoundary: "./live",
				Filters:           parseFilters("...[main...HEAD]"),
			},
			expected: "live",
		},
		{
			name: "flag does not narrow a filter without dependents",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir:        repoRoot,
				DiscoveryBoundary: "./live",
				Filters:           parseFilters("[main...HEAD]"),
			},
			expected: "",
		},
		{
			name:     "dependency-side boundary does not narrow",
			v:        v,
			opts:     discovery.StackGenerateOptions{WorkingDir: repoRoot, Filters: parseFilters("[main...HEAD]...(./live)")},
			expected: "",
		},
		{
			name: "disjoint positive boundaries do not narrow",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir: repoRoot,
				Filters:    parseFilters("(./live/staging)...[main...HEAD]", "(./live/production)...[main...HEAD]"),
			},
			expected: "",
		},
		{
			name: "boundary above the repository root covers it",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir: repoRoot,
				Filters:    parseFilters("(..)...[main...HEAD]"),
			},
			expected: "",
		},
		{
			name: "nested positive boundaries collapse to the outermost",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir: repoRoot,
				Filters:    parseFilters("(./live/staging)...[main...HEAD]", "(./live)...[main...HEAD]"),
			},
			expected: "live",
		},
		{
			name: "Git boundary beside the working directory",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir: stagingDir,
				Filters:    parseFilters("(./live/production)...[main...HEAD]"),
			},
			expected: filepath.Join("live", "production"),
		},
		{
			name: "flag boundary is mirrored even when missing locally",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir:        repoRoot,
				DiscoveryBoundary: filepath.Join(repoRoot, "missing"),
			},
			expected: "missing",
		},
		{
			name: "outside a git repository",
			v: venvtest.New().WithFS(venvtest.NewFS(t, repoRoot, map[string]string{
				filepath.Join("live", ".keep"): "",
			})),
			opts:     discovery.StackGenerateOptions{WorkingDir: repoRoot, DiscoveryBoundary: liveDir},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, discovery.WorktreeBoundary(t.Context(), logger.CreateLogger(), tc.v, tc.opts))
		})
	}
}

func TestWorktreeWalkRoot(t *testing.T) {
	t.Parallel()

	worktree := venvtest.Root("/worktree")
	fsys := venvtest.NewFS(t, worktree, map[string]string{
		filepath.Join("live", ".keep"): "",
		"file":                         "",
	})

	testCases := []struct {
		fsys     vfs.FS
		errIs    error
		name     string
		boundary string
		expected string
		ok       bool
	}{
		{name: "existing directory", fsys: fsys, boundary: "live", expected: filepath.Join(worktree, "live"), ok: true},
		{name: "missing at this ref", fsys: fsys, boundary: "missing"},
		{name: "file at this ref", fsys: fsys, boundary: "file"},
		{name: "beneath a file at this ref", fsys: fsys, boundary: filepath.Join("file", "live")},
		{
			name:     "unreadable boundary",
			fsys:     statErrFS{FS: fsys, err: fs.ErrPermission},
			boundary: "live",
			errIs:    fs.ErrPermission,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root, ok, err := discovery.WorktreeWalkRoot(tc.fsys, worktree, tc.boundary)
			if tc.errIs != nil {
				require.ErrorIs(t, err, tc.errIs)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.expected, root)
		})
	}
}

func TestWithinWorktreeBoundary(t *testing.T) {
	t.Parallel()

	worktree := venvtest.Root("/worktree")
	fsys := venvtest.NewFS(t, worktree, map[string]string{
		filepath.Join("live", "app", "terragrunt.stack.hcl"):    "",
		filepath.Join("catalog", "app", "terragrunt.stack.hcl"): "",
	})

	inWorktree := func(path string) component.Component {
		return component.NewStack(filepath.Join(worktree, path)).WithDiscoveryContext(
			&component.DiscoveryContext{WorkingDir: worktree, Ref: "HEAD"},
		)
	}

	testCases := []struct {
		name      string
		component component.Component
		boundary  string
		expected  bool
	}{
		{
			name:      "no boundary",
			component: inWorktree(filepath.Join("catalog", "app")),
			expected:  true,
		},
		{
			name:      "inside the boundary",
			component: inWorktree(filepath.Join("live", "app")),
			boundary:  "live",
			expected:  true,
		},
		{
			name:      "outside the boundary",
			component: inWorktree(filepath.Join("catalog", "app")),
			boundary:  "live",
			expected:  false,
		},
		{
			name:      "no discovery context",
			component: component.NewStack(filepath.Join(worktree, "catalog", "app")),
			boundary:  "live",
			expected:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, discovery.WithinWorktreeBoundary(fsys, tc.component, tc.boundary))
		})
	}
}

// statErrFS fails every Stat with err.
type statErrFS struct {
	vfs.FS
	err error
}

func (f statErrFS) Stat(string) (fs.FileInfo, error) {
	return nil, f.err
}

func TestCheckWorktreeBoundaries(t *testing.T) {
	t.Parallel()

	from := venvtest.Root("/from")
	to := venvtest.Root("/to")
	other := venvtest.Root("/other")
	// Git answers for a reference that is not checked out: main also holds live/tree-only.
	v := venvtest.New().WithFS(venvtest.NewFS(t, venvtest.Root("/"), map[string]string{
		filepath.Join("from", "live", "old", ".keep"): "",
		filepath.Join("to", "live", "new", ".keep"):   "",
		filepath.Join("other", "catalog", ".keep"):    "",
		filepath.Join("repo", ".git"):                 "gitdir: /elsewhere/.git\n",
	})).WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if slices.Contains(inv.Args, "ls-tree") && slices.Contains(inv.Args, "main") {
			return vexec.Result{Stdout: []byte("live/old/terragrunt.hcl\x00live/tree-only/terragrunt.hcl\x00")}
		}

		return vexec.Result{Stdout: []byte("")}
	})

	pairs := func(fromPath string) *worktrees.Worktrees {
		return &worktrees.Worktrees{
			OriginalWorkingDir: venvtest.Root("/repo"),
			WorktreePairs: map[string]*worktrees.WorktreePair{
				"[main...HEAD]": {
					FromWorktree: worktrees.Worktree{Ref: "main", Path: fromPath},
					ToWorktree:   worktrees.Worktree{Ref: "HEAD", Path: to},
				},
				"[HEAD~1...HEAD~2]": {
					FromWorktree: worktrees.Worktree{Ref: "HEAD~1", Path: other},
					ToWorktree:   worktrees.Worktree{Ref: "HEAD~2", Path: other},
				},
			},
		}
	}

	parseFilters := func(queries ...string) filter.Filters {
		filters, err := filter.ParseFilterQueries(logger.CreateLogger(), queries)
		require.NoError(t, err)

		return filters
	}

	testCases := []struct {
		worktrees *worktrees.Worktrees
		name      string
		flag      string
		filters   filter.Filters
		wantErr   bool
	}{
		{
			name:    "no worktrees",
			filters: parseFilters("(./nope)...[main...HEAD]"),
		},
		{
			name:      "dependent boundary only at the to reference",
			worktrees: pairs(from),
			filters:   parseFilters("(./live/new)...[main...HEAD]"),
		},
		{
			name:      "dependent boundary only at the from reference",
			worktrees: pairs(from),
			filters:   parseFilters("(./live/old)...[main...HEAD]"),
		},
		{
			name:      "dependent boundary at neither reference",
			worktrees: pairs(from),
			filters:   parseFilters("(./nope)...[main...HEAD]"),
			wantErr:   true,
		},
		{
			name:      "dependency boundary at neither reference",
			worktrees: pairs(from),
			filters:   parseFilters("[main...HEAD]...(./nope)"),
			wantErr:   true,
		},
		{
			name:      "boundary above the repository root covers it",
			worktrees: pairs(from),
			filters:   parseFilters("(..)...[main...HEAD]"),
		},
		{
			name:      "flag above the repository root covers it",
			worktrees: pairs(from),
			filters:   parseFilters("...[main...HEAD]"),
			flag:      venvtest.Root("/"),
		},
		{
			name:      "boundary naming a file is not a directory",
			worktrees: pairs(from),
			filters:   parseFilters("(./live/new/.keep)...[main...HEAD]"),
			wantErr:   true,
		},
		{
			name:      "non-Git boundary is not checked",
			worktrees: pairs(from),
			filters:   parseFilters("(./nope)...{./app}"),
		},
		{
			name:      "flag at neither reference for a dependents filter",
			worktrees: pairs(from),
			filters:   parseFilters("...[main...HEAD]"),
			flag:      "./nope",
			wantErr:   true,
		},
		{
			name:      "flag is not checked for a filter without traversal",
			worktrees: pairs(from),
			filters:   parseFilters("[main...HEAD]"),
			flag:      "./nope",
		},
		{
			name:      "skipped reference answers from its Git tree",
			worktrees: pairs(""),
			filters:   parseFilters("(./live/tree-only)...[main...HEAD]"),
		},
		{
			name:      "boundary missing from a skipped reference and the checkout",
			worktrees: pairs(""),
			filters:   parseFilters("(./nope)...[main...HEAD]"),
			wantErr:   true,
		},
		{
			name:      "each boundary is checked at its own references",
			worktrees: pairs(from),
			filters:   parseFilters("(./live/new)...[main...HEAD]", "(./catalog)...[HEAD~1...HEAD~2]"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := discovery.CheckWorktreeBoundaries(t.Context(), v, tc.worktrees, tc.filters, tc.flag, venvtest.Root("/repo"))
			if tc.wantErr {
				require.ErrorAs(t, err, &discovery.DiscoveryBoundaryDirError{})
				return
			}

			require.NoError(t, err)
		})
	}
}
