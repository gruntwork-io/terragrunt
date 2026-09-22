package discovery_test

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/shell/split"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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

	v := memRepoRootVenv(t, repoRoot)

	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(stagingDir, ".keep"), nil, 0o644))

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
			name: "inline boundary relative to a nested working directory",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir: liveDir,
				Filters:    parseFilters("(./staging)...[main...HEAD]"),
			},
			expected: filepath.Join("live", "staging"),
		},
		{
			name:     "boundary equal to the git root",
			v:        v,
			opts:     discovery.StackGenerateOptions{WorkingDir: repoRoot, DiscoveryBoundary: repoRoot},
			expected: "",
		},
		{
			name:     "boundary wider than the working directory clips to it",
			v:        v,
			opts:     discovery.StackGenerateOptions{WorkingDir: stagingDir, DiscoveryBoundary: liveDir},
			expected: filepath.Join("live", "staging"),
		},
		{
			name:     "git root boundary clips to a nested working directory",
			v:        v,
			opts:     discovery.StackGenerateOptions{WorkingDir: stagingDir, DiscoveryBoundary: repoRoot},
			expected: filepath.Join("live", "staging"),
		},
		{
			name:     "inline parent boundary clips to the working directory",
			v:        v,
			opts:     discovery.StackGenerateOptions{WorkingDir: stagingDir, Filters: parseFilters("(..)...[main...HEAD]")},
			expected: filepath.Join("live", "staging"),
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
			name: "nonexistent boundary",
			v:    v,
			opts: discovery.StackGenerateOptions{
				WorkingDir:        repoRoot,
				DiscoveryBoundary: filepath.Join(repoRoot, "missing"),
			},
			expected: "",
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

			assert.Equal(t, tc.expected, discovery.WorktreeBoundary(t.Context(), tc.v, tc.opts))
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
