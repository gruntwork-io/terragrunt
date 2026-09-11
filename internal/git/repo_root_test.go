package git_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

const fixtureDirPerm = 0755

// repoRootFS lays out an in-memory tree: an outer repository with a unit, and
// a nested repository, vendored inside it, with a unit of its own. `.git` is
// written as a file, the shape a submodule and a linked worktree both use.
func repoRootFS(t *testing.T) map[string]string {
	t.Helper()

	return map[string]string{
		".git":                         "gitdir: /elsewhere/.git/modules/outer\n",
		"live/prod/vpc/terragrunt.hcl": "",
		"vendor/mod/.git":              "gitdir: /elsewhere/.git/modules/mod\n",
		"vendor/mod/live/vpc/main.tf":  "",
		"vendor/mod/README.md":         "",
	}
}

func TestGoRepoRootWalksToNearestGit(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/outer")
	v := venvtest.New().WithFS(venvtest.NewFS(t, root, repoRootFS(t)))

	got, err := git.GoRepoRoot(
		cache.ContextWithCache(t.Context()),
		v,
		filepath.Join(root, "live", "prod", "vpc"),
	)
	require.NoError(t, err)
	assert.Equal(t, root, got)
}

// TestGoRepoRootPrefersNestedRepo pins the answer for a unit inside a vendored
// repository, in both resolution orders. Resolving the outer unit first
// memoizes directories that enclose the nested repository, and the nested unit
// must still resolve to the nested root.
func TestGoRepoRootPrefersNestedRepo(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/outer")
	nested := filepath.Join(root, "vendor", "mod")

	for _, tc := range []struct {
		name  string
		order []string
	}{
		{name: "nested first", order: []string{nested, root}},
		{name: "outer first", order: []string{root, nested}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New().WithFS(venvtest.NewFS(t, root, repoRootFS(t)))
			ctx := cache.ContextWithCache(t.Context())

			for _, start := range tc.order {
				unit := filepath.Join(start, "live", "vpc")
				if start == root {
					unit = filepath.Join(root, "live", "prod", "vpc")
				}

				got, err := git.GoRepoRoot(ctx, v, unit)
				require.NoError(t, err)
				assert.Equal(t, start, got, "unit %q", unit)
			}
		})
	}
}

// TestGoRepoRootMemoizesEveryWalkedDirectory pins that a walk records the
// whole chain it proved free of a `.git`, not just the directory it was asked
// about. That is what lets a sibling unit stop at the first memoized ancestor
// instead of re-walking to the root.
func TestGoRepoRootMemoizesEveryWalkedDirectory(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/outer")
	v := venvtest.New().WithFS(venvtest.NewFS(t, root, repoRootFS(t)))

	ctx := cache.ContextWithCache(t.Context())
	roots := cache.ContextRepoRootCache(ctx, cache.RepoRootCacheContextKey)

	_, err := git.GoRepoRoot(ctx, v, filepath.Join(root, "live", "prod", "vpc"))
	require.NoError(t, err)

	// live, live/prod and live/prod/vpc were each proved free of a `.git`.
	assert.Equal(t, 3, roots.Len())

	for _, dir := range []string{"live", "live/prod", "live/prod/vpc"} {
		cached, ok := roots.Lookup(ctx, filepath.Join(root, filepath.FromSlash(dir)))
		assert.True(t, ok, dir)
		assert.Equal(t, root, cached, dir)
	}
}

func TestGoRepoRootOutsideAnyRepo(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/elsewhere")
	v := venvtest.New().WithFS(venvtest.NewFS(t, root, map[string]string{"unit/main.tf": ""}))

	_, err := git.GoRepoRoot(cache.ContextWithCache(t.Context()), v, filepath.Join(root, "unit"))
	require.ErrorIs(t, err, git.ErrNoRepoRoot)
}

// TestGoRepoRootRejectsRelativePath pins the contract that a relative path is
// refused rather than resolved against the process working directory.
func TestGoRepoRootRejectsRelativePath(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/outer")
	v := venvtest.New().WithFS(venvtest.NewFS(t, root, repoRootFS(t)))

	for _, path := range []string{"", ".", filepath.FromSlash("live/prod/vpc")} {
		_, err := git.GoRepoRoot(cache.ContextWithCache(t.Context()), v, path)
		require.ErrorIs(t, err, git.ErrRepoPathNotAbsolute, "path %q", path)
	}
}

// TestGoRepoRootCeilingDirectories pins GIT_CEILING_DIRECTORIES against the
// rules git's own discovery follows. The fixture puts the repository root two
// levels above the unit, so a ceiling can sit between them, on them, or
// nowhere near them.
func TestGoRepoRootCeilingDirectories(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/outer")
	unit := filepath.Join(root, "live", "prod", "vpc")
	mid := filepath.Join(root, "live")

	for _, tc := range []struct {
		name    string
		ceiling string
		want    string
		blocked bool
	}{
		{name: "unset", ceiling: "", want: root},
		{name: "blocks at the repository root", ceiling: root, blocked: true},
		{name: "blocks below the repository root", ceiling: mid, blocked: true},
		{
			name:    "starting directory is exempt",
			ceiling: unit,
			want:    root,
		},
		{
			name:    "unrelated entry does not apply",
			ceiling: filepath.FromSlash("/elsewhere"),
			want:    root,
		},
		{
			name:    "relative entries are dropped",
			ceiling: filepath.FromSlash("../.."),
			want:    root,
		},
		{
			name:    "deepest blocking entry wins",
			ceiling: strings.Join([]string{filepath.FromSlash("/elsewhere"), mid}, string(filepath.ListSeparator)),
			blocked: true,
		},
		{
			// The empty entry is git's opt-out from resolving symlinks in the
			// rest of the list, not an entry of its own. What resolution then
			// does to a real symlinked path is covered against the binary in
			// TestExecGoRepoRootCeilingMatchesGit.
			name:    "leading empty entry is not itself an entry",
			ceiling: strings.Join([]string{"", mid}, string(filepath.ListSeparator)),
			blocked: true,
		},
		{
			name:    "sibling sharing a prefix is not an ancestor",
			ceiling: root + "bar",
			want:    root,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New().
				WithFS(venvtest.NewFS(t, root, repoRootFS(t))).
				WithEnv(map[string]string{git.EnvNameGitCeilingDirectories: tc.ceiling})

			got, err := git.GoRepoRoot(cache.ContextWithCache(t.Context()), v, unit)

			if tc.blocked {
				require.ErrorIs(t, err, git.ErrNoRepoRoot)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestGoRepoRootCeilingOutranksMemo pins the ordering the walk depends on.
// Resolving the ceiling directory itself is exempt, and memoizes it against
// the root above. A unit below the ceiling must still be refused rather than
// served that memoized answer, which is why the ceiling is checked before the
// cache is consulted.
func TestGoRepoRootCeilingOutranksMemo(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/outer")
	ceiling := filepath.Join(root, "live")
	unit := filepath.Join(ceiling, "prod", "vpc")

	v := venvtest.New().
		WithFS(venvtest.NewFS(t, root, repoRootFS(t))).
		WithEnv(map[string]string{git.EnvNameGitCeilingDirectories: ceiling})

	ctx := cache.ContextWithCache(t.Context())

	// The ceiling directory is its own starting point, so the walk from it is
	// unblocked and records it in the memo.
	got, err := git.GoRepoRoot(ctx, v, ceiling)
	require.NoError(t, err)
	require.Equal(t, root, got)

	_, err = git.GoRepoRoot(ctx, v, unit)
	require.ErrorIs(t, err, git.ErrNoRepoRoot)
}

// TestGoRepoRootConcurrentResolutionWithRacing resolves many units against one
// shared memo the way discovery does, which parses components through an
// errgroup and evaluates get_repo_root() inside each worker. The fixture is on
// disk so a race the detector reports belongs to this package rather than to
// an in-memory filesystem.
func TestGoRepoRootConcurrentResolutionWithRacing(t *testing.T) {
	t.Parallel()

	const (
		units   = 64
		readers = 4
	)

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), fixtureDirPerm))

	unitDirs := make([]string, units)

	for i := range unitDirs {
		unitDirs[i] = filepath.Join(root, "live", "prod", "unit-"+strconv.Itoa(i))
		require.NoError(t, os.MkdirAll(unitDirs[i], fixtureDirPerm))
	}

	want := vfs.ResolveForCompare(vfs.NewOSFS(), root)

	ctx := cache.ContextWithCache(t.Context())
	v := venvtest.NewWithOSFS()

	g, ctx := errgroup.WithContext(ctx)

	// Several readers per unit, so the same directory is resolved concurrently
	// as well as different ones.
	for range readers {
		for _, unitDir := range unitDirs {
			g.Go(func() error {
				got, err := git.GoRepoRoot(ctx, v, unitDir)
				if err != nil {
					return err
				}

				if got != want {
					return fmt.Errorf("resolving %q: got %q, want %q", unitDir, got, want)
				}

				return nil
			})
		}
	}

	require.NoError(t, g.Wait())
}

// statErrorFS fails Stat for one path, standing in for a `.git` entry the
// process cannot read.
type statErrorFS struct {
	vfs.FS
	err     error
	failFor string
}

func (fsys *statErrorFS) Stat(name string) (os.FileInfo, error) {
	if name == fsys.failFor {
		return nil, fsys.err
	}

	return fsys.FS.Stat(name)
}

// TestGoRepoRootSurfacesUnreadableGitEntry pins that a `.git` entry the
// process cannot stat is reported rather than read as absent. Treating it as
// absent would walk past a repository and answer with an enclosing one.
func TestGoRepoRootSurfacesUnreadableGitEntry(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/outer")
	unit := filepath.Join(root, "live", "prod", "vpc")

	fsys := &statErrorFS{
		FS:      venvtest.NewFS(t, root, repoRootFS(t)),
		failFor: filepath.Join(root, ".git"),
		err:     os.ErrPermission,
	}

	_, err := git.GoRepoRoot(cache.ContextWithCache(t.Context()), venvtest.New().WithFS(fsys), unit)
	require.ErrorIs(t, err, os.ErrPermission)
	require.NotErrorIs(t, err, git.ErrNoRepoRoot)
}

// TestGoRepoRootWalkDepthBound pins the walk's depth bound, injected so the
// test does not have to build a tree as deep as the default. The fixture puts
// the unit four directories below the root, so a bound below four aborts and
// one at four reaches it.
func TestGoRepoRootWalkDepthBound(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/outer")
	unit := filepath.Join(root, "live", "prod", "vpc")

	for _, tc := range []struct {
		name    string
		depth   int
		aborted bool
	}{
		{name: "one short", depth: 3, aborted: true},
		{name: "exact", depth: 4},
		{name: "generous", depth: 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New().WithFS(venvtest.NewFS(t, root, repoRootFS(t)))

			got, err := git.GoRepoRoot(
				cache.ContextWithCache(t.Context()),
				v,
				unit,
				git.WithMaxWalkDepth(tc.depth),
			)

			if tc.aborted {
				require.ErrorIs(t, err, git.ErrRepoRootWalkDepth)
				assert.NotErrorIs(t, err, git.ErrNoRepoRoot,
					"an aborted walk must not read as a path outside any repository")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, root, got)
		})
	}
}

// TestGoRepoRootDefaultWalkDepth pins that omitting the option leaves a bound
// generous enough for a real tree, so the injected one is only ever narrowing.
func TestGoRepoRootDefaultWalkDepth(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/outer")
	v := venvtest.New().WithFS(venvtest.NewFS(t, root, repoRootFS(t)))

	got, err := git.GoRepoRoot(
		cache.ContextWithCache(t.Context()),
		v,
		filepath.Join(root, "live", "prod", "vpc"),
	)
	require.NoError(t, err)
	assert.Equal(t, root, got)
}
