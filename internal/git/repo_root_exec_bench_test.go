//go:build exec

// Benchmarks and real-git parity checks for [git.GoRepoRoot]. The fork arm
// spawns the git binary GoRepoRoot replaced, so the whole file sits behind the
// exec tag alongside the other real-binary suites.

package git_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const maxForkBenchUnits = 100

// benchRunDepth is how deep a unit sits under the repository root in the run
// benchmark, in the range a `live/<env>/<region>/<unit>` layout produces.
const benchRunDepth = 4

// forkRepoRoot is the implementation GoRepoRoot replaced, kept as the
// benchmark's baseline and as the reference the parity tests compare against.
// Each entry in env is added to the git process's own environment.
func forkRepoRoot(ctx context.Context, dir string, env ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir

	cmd.Env = append(os.Environ(), env...)

	var stdout bytes.Buffer

	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return filepath.FromSlash(strings.TrimSpace(stdout.String())), nil
}

// BenchmarkRepoRootSingle measures one unmemoized resolution: what a run pays
// the first time it needs the repository root. Depth is how many directories
// separate the starting point from the root.
func BenchmarkRepoRootSingle(b *testing.B) {
	requireGit(b)

	v := venv.OSVenv()

	for _, depth := range []int{1, 5, 10} {
		unitDir := benchGitRepo(b, depth, 1)[0]
		name := "depth=" + strconv.Itoa(depth)

		b.Run(name+"/fork", func(b *testing.B) {
			for b.Loop() {
				if _, err := forkRepoRoot(b.Context(), unitDir); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(name+"/go", func(b *testing.B) {
			for b.Loop() {
				// A fresh cache per op: the memo is per run, so the first
				// resolution never starts warm.
				if _, err := git.GoRepoRoot(
					cache.ContextWithCache(b.Context()),
					v,
					unitDir,
				); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkRepoRootRun measures the shape a `run --all` produces: one
// resolution per unit, every unit sitting benchRunDepth levels below a single
// repository root.
//
// The go-cached arm is production's shape, one run-scoped cache for the whole
// iteration. The go-uncached arm gives every unit a fresh cache, which is what
// the memo is worth: it is the same walk with nothing carried between units.
func BenchmarkRepoRootRun(b *testing.B) {
	requireGit(b)

	v := venv.OSVenv()

	for _, units := range []int{10, 100, 1000, 10000} {
		unitDirs := benchGitRepo(b, benchRunDepth, units)
		name := "units=" + strconv.Itoa(units)

		// The fork baseline costs the same per unit at every count, so the
		// larger ones would spend minutes restating a linear cost.
		if units <= maxForkBenchUnits {
			b.Run(name+"/fork", func(b *testing.B) {
				for b.Loop() {
					for _, unitDir := range unitDirs {
						if _, err := forkRepoRoot(b.Context(), unitDir); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}

		b.Run(name+"/go-cached", func(b *testing.B) {
			for b.Loop() {
				ctx := cache.ContextWithCache(b.Context())

				for _, unitDir := range unitDirs {
					if _, err := git.GoRepoRoot(ctx, v, unitDir); err != nil {
						b.Fatal(err)
					}
				}
			}
		})

		b.Run(name+"/go-uncached", func(b *testing.B) {
			for b.Loop() {
				for _, unitDir := range unitDirs {
					if _, err := git.GoRepoRoot(
						cache.ContextWithCache(b.Context()),
						v,
						unitDir,
					); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// TestExecGoRepoRootMatchesGit pins GoRepoRoot against the real git binary on
// the layouts a Terragrunt run meets: a plain checkout, the root itself, a
// linked worktree and a submodule (both of which carry a `.git` file rather
// than a directory), and a path reached through a symlink.
func TestExecGoRepoRootMatchesGit(t *testing.T) {
	t.Parallel()
	requireGit(t)

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(repo, fixtureDirPerm))
	initGitRepo(t, repo)

	nested := filepath.Join(repo, "live", "prod", "vpc")
	require.NoError(t, os.MkdirAll(nested, fixtureDirPerm))

	sub := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(sub, fixtureDirPerm))
	initGitRepo(t, sub)
	runGit(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", sub, "vendored")

	worktree := filepath.Join(root, "wt")
	runGit(t, repo, "worktree", "add", worktree)

	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(nested, link))

	ctx := cache.ContextWithCache(t.Context())
	v := venv.OSVenv()

	for _, dir := range []string{repo, nested, worktree, filepath.Join(repo, "vendored"), link} {
		got, err := git.GoRepoRoot(ctx, v, dir)
		require.NoError(t, err, dir)
		require.Equal(t, gitRepoRoot(t, dir), got, dir)
	}
}

// TestExecGoRepoRootCeilingMatchesGit runs the ceiling rules past the real
// binary. The mem-backed tests pin the same cases, but only git can say
// whether the rules themselves were read correctly: that the starting
// directory is exempt, that a ceiling blocks the directory it names rather
// than only the ones above it, and that relative entries are dropped.
func TestExecGoRepoRootCeilingMatchesGit(t *testing.T) {
	t.Parallel()
	requireGit(t)

	repo := t.TempDir()
	initGitRepo(t, repo)

	unit := filepath.Join(repo, "live", "prod", "vpc")
	require.NoError(t, os.MkdirAll(unit, fixtureDirPerm))

	mid := filepath.Join(repo, "live")

	for _, ceiling := range []string{
		"",
		repo,
		mid,
		unit,
		filepath.Join(repo, "live", "prod"),
		t.TempDir(),
		filepath.FromSlash("../.."),
		strings.Join([]string{t.TempDir(), mid}, string(filepath.ListSeparator)),
		strings.Join([]string{"", mid}, string(filepath.ListSeparator)),
	} {
		t.Run("ceiling="+ceiling, func(t *testing.T) {
			t.Parallel()

			v := venv.OSVenv().WithEnv(map[string]string{git.EnvNameGitCeilingDirectories: ceiling})

			want, gitErr := forkRepoRoot(
				t.Context(),
				unit,
				git.EnvNameGitCeilingDirectories+"="+ceiling,
			)
			got, err := git.GoRepoRoot(cache.ContextWithCache(t.Context()), v, unit)

			if gitErr != nil {
				require.ErrorIs(t, err, git.ErrNoRepoRoot,
					"git refused, so the walk must too")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

// TestExecGoRepoRootNestedOrdering is the case the cache's shape exists for.
// Resolving a unit in the outer repository first memoizes directories that
// enclose the nested repository, so a later query for a unit inside the nested
// one must still answer with the nested root. Per-directory entries hold that
// line; prefix containment over cached roots would not.
func TestExecGoRepoRootNestedOrdering(t *testing.T) {
	t.Parallel()
	requireGit(t)

	outer := t.TempDir()
	initGitRepo(t, outer)

	outerUnit := filepath.Join(outer, "live", "vpc")
	require.NoError(t, os.MkdirAll(outerUnit, fixtureDirPerm))

	inner := filepath.Join(outer, "vendor", "module")
	require.NoError(t, os.MkdirAll(inner, fixtureDirPerm))
	initGitRepo(t, inner)

	innerUnit := filepath.Join(inner, "live", "vpc")
	require.NoError(t, os.MkdirAll(innerUnit, fixtureDirPerm))

	ctx := cache.ContextWithCache(t.Context())
	v := venv.OSVenv()

	for _, dir := range []string{outerUnit, innerUnit, outerUnit} {
		got, err := git.GoRepoRoot(ctx, v, dir)
		require.NoError(t, err, dir)
		require.Equal(t, gitRepoRoot(t, dir), got, dir)
	}
}

// TestExecGoRepoRootOutsideRepo pins that the walk reports a typed miss where
// git exits non-zero, rather than climbing out to a repository that happens to
// enclose the temp directory.
func TestExecGoRepoRootOutsideRepo(t *testing.T) {
	t.Parallel()
	requireGit(t)

	dir := t.TempDir()

	_, gitErr := forkRepoRoot(t.Context(), dir)
	require.Error(t, gitErr, "fixture must sit outside any repository for this to mean anything")

	_, err := git.GoRepoRoot(cache.ContextWithCache(t.Context()), venv.OSVenv(), dir)
	require.ErrorIs(t, err, git.ErrNoRepoRoot)
}

// TestExecGoRepoRootBareRepository pins what a bare repository resolves to.
// Git refuses to report a toplevel there, and the walk reaches the same
// outcome by a different route: a bare repository holds no `.git` entry, so
// nothing about it stops the walk.
func TestExecGoRepoRootBareRepository(t *testing.T) {
	t.Parallel()
	requireGit(t)

	bare := filepath.Join(t.TempDir(), "repo.git")
	require.NoError(t, os.MkdirAll(bare, fixtureDirPerm))
	runGit(t, bare, "init", "--bare")

	_, gitErr := forkRepoRoot(t.Context(), bare)
	require.Error(t, gitErr, "git must refuse a bare repository for this to mean anything")

	_, err := git.GoRepoRoot(cache.ContextWithCache(t.Context()), venv.OSVenv(), bare)
	require.ErrorIs(t, err, git.ErrNoRepoRoot)
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
		require.NoError(tb, os.MkdirAll(unitDirs[i], fixtureDirPerm))
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

// gitRepoRoot is the reference answer: what the subprocess resolver returned.
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
