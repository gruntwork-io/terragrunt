//go:build exec

// Benchmark and parity check for resolving a git repository root without
// forking git. Every measurement here spawns the real git binary, so the file
// sits behind the exec tag alongside the other real-binary suites.

package shell_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// maxRepoRootWalkDepth caps the upward walk so a path whose Dir() never
// reaches a fixed point cannot hang the process.
const maxRepoRootWalkDepth = 1024

var (
	// errNotARepo stands in for git's "not a git repository" exit.
	errNotARepo = errors.New("no enclosing git repository")

	// errWalkDepthExceeded reports an aborted walk, so a caller cannot read a
	// truncated scan as a clean miss.
	errWalkDepthExceeded = errors.New("repo-root walk exceeded max depth")
)

// findRepoRootInline is the candidate replacement for `git rev-parse
// --show-toplevel`: walk up from dir until a `.git` entry appears. A `.git`
// file (linked worktree, submodule) counts the same as a directory, which is
// what makes the plain walk agree with git on those layouts.
func findRepoRootInline(fsys vfs.FS, dir string) (string, error) {
	current := vfs.ResolveForCompare(fsys, filepath.Clean(dir))

	for range maxRepoRootWalkDepth {
		_, err := fsys.Stat(filepath.Join(current, ".git"))
		if err == nil {
			return current, nil
		}

		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", errNotARepo
		}

		current = parent
	}

	return "", errWalkDepthExceeded
}

// BenchmarkRepoRootSingle measures one uncached resolution: what a run pays
// the first time it needs the repository root, and what every later call would
// cost if the memoization went away. Depth is how many directories separate
// the starting point from the root.
func BenchmarkRepoRootSingle(b *testing.B) {
	requireGit(b)

	l := benchLogger()
	v := venv.OSVenv()

	for _, depth := range []int{1, 5, 10} {
		unitDir := benchGitRepo(b, depth, 1)[0]
		name := "depth=" + strconv.Itoa(depth)

		b.Run(name+"/fork", func(b *testing.B) {
			for b.Loop() {
				// A fresh cache per op: the memo is per run, so the first
				// resolution never starts warm.
				if _, err := shell.GitTopLevelDir(cache.ContextWithCache(b.Context()), l, v, unitDir); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(name+"/inline", func(b *testing.B) {
			for b.Loop() {
				if _, err := findRepoRootInline(v.FS, unitDir); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkRepoRootRun measures the shape a `run --all` produces: one
// resolution per unit, every unit sitting depth levels below a single
// repository root.
//
// The fork arm carries the run-scoped cache it has today, so one iteration
// pays a single fork plus a guarded cache lookup for every other unit. The
// inline arm carries no cache at all, because the walk needs none: it finds
// the nearest `.git` from the unit itself, which is what the cache's
// nested-repository guard exists to re-check.
func BenchmarkRepoRootRun(b *testing.B) {
	requireGit(b)

	l := benchLogger()
	v := venv.OSVenv()

	for _, units := range []int{10, 100} {
		unitDirs := benchGitRepo(b, benchRunDepth, units)
		name := "units=" + strconv.Itoa(units)

		b.Run(name+"/fork", func(b *testing.B) {
			for b.Loop() {
				ctx := cache.ContextWithCache(b.Context())

				for _, unitDir := range unitDirs {
					if _, err := shell.GitTopLevelDir(ctx, l, v, unitDir); err != nil {
						b.Fatal(err)
					}
				}
			}
		})

		b.Run(name+"/inline", func(b *testing.B) {
			for b.Loop() {
				for _, unitDir := range unitDirs {
					if _, err := findRepoRootInline(v.FS, unitDir); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// benchRunDepth is how deep a unit sits under the repository root in the run
// benchmark, in the range a `live/<env>/<region>/<unit>` layout produces.
const benchRunDepth = 4

// benchLogger discards output at error level, so the debug lines the fork path
// writes for every resolution do not land in the measurement.
func benchLogger() log.Logger {
	return logger.CreateLogger().WithOptions(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel))
}

// TestExecInlineRepoRootMatchesGit pins the inline walk against the real git
// binary on the layouts a Terragrunt run meets: a plain checkout, the root
// itself, a linked worktree and a submodule (both of which carry a `.git`
// file rather than a directory), and a path reached through a symlink.
func TestExecInlineRepoRootMatchesGit(t *testing.T) {
	t.Parallel()
	requireGit(t)

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(repo, 0755))
	initGitRepo(t, repo)

	nested := filepath.Join(repo, "live", "prod", "vpc")
	require.NoError(t, os.MkdirAll(nested, 0755))

	sub := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(sub, 0755))
	initGitRepo(t, sub)
	runGit(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", sub, "vendored")

	worktree := filepath.Join(root, "wt")
	runGit(t, repo, "worktree", "add", worktree)

	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(nested, link))

	fsys := vfs.NewOSFS()

	for _, dir := range []string{repo, nested, worktree, filepath.Join(repo, "vendored"), link} {
		got, err := findRepoRootInline(fsys, dir)
		require.NoError(t, err, dir)
		require.Equal(t, gitRepoRoot(t, dir), got, dir)
	}
}

// TestExecInlineRepoRootOutsideRepo pins that the walk reports a miss where
// git exits non-zero, rather than climbing out to a repository that happens to
// enclose the temp directory.
func TestExecInlineRepoRootOutsideRepo(t *testing.T) {
	t.Parallel()
	requireGit(t)

	_, err := findRepoRootInline(vfs.NewOSFS(), t.TempDir())
	require.ErrorIs(t, err, errNotARepo)
}

// benchGitRepo builds a repository holding units unit directories, each depth
// levels below the root, and returns their paths.
func benchGitRepo(tb testing.TB, depth, units int) []string {
	tb.Helper()

	repo := tb.TempDir()
	initGitRepo(tb, repo)

	parent := repo
	for i := range depth {
		parent = filepath.Join(parent, "level-"+strconv.Itoa(i))
	}

	unitDirs := make([]string, units)

	for i := range units {
		unitDirs[i] = filepath.Join(parent, "unit-"+strconv.Itoa(i))
		require.NoError(tb, os.MkdirAll(unitDirs[i], 0755))
	}

	return unitDirs
}

// initGitRepo creates a repository with one commit, which a linked worktree
// and a submodule both need.
func initGitRepo(tb testing.TB, dir string) {
	tb.Helper()

	runGit(tb, dir, "init")
	runGit(tb, dir, "config", "user.email", "bench@example.com")
	runGit(tb, dir, "config", "user.name", "bench")
	runGit(tb, dir, "commit", "--allow-empty", "-m", "root")
}

// gitRepoRoot is the reference answer: what the subprocess resolver returns
// today.
func gitRepoRoot(tb testing.TB, dir string) string {
	tb.Helper()

	return filepath.FromSlash(strings.TrimSpace(runGit(tb, dir, "rev-parse", "--show-toplevel")))
}

func runGit(tb testing.TB, dir string, args ...string) string {
	tb.Helper()

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	require.NoError(tb, err, string(out))

	return string(out)
}

func requireGit(tb testing.TB) {
	tb.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		tb.Skip("git binary not installed on this host")
	}
}
