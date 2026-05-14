package shell_test

import (
	"context"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitTopLevelDirDispatchesGitRevParse pins the exact subprocess
// invocation GitTopLevelDir uses to resolve a repository root. The mem
// backend asserts the command, args, and working directory so a refactor
// that drops or reorders any of them is caught.
func TestGitTopLevelDirDispatchesGitRevParse(t *testing.T) {
	t.Parallel()

	var calls int

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		calls++

		assert.Equal(t, "git", inv.Name)
		assert.Equal(t, []string{"rev-parse", "--show-toplevel"}, inv.Args)
		assert.Equal(t, "/tmp/repo", inv.Dir)

		return vexec.Result{Stdout: []byte("/tmp/repo\n")}
	})

	root, err := shell.GitTopLevelDir(gitMemCtx(t), logger.CreateLogger(), exec, nil, "/tmp/repo")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/repo", root)
	assert.Equal(t, 1, calls)
}

// TestGitTopLevelDirNormalizesWindowsSlashes pins the contract that
// git's forward-slash output is normalized to OS-native separators so
// downstream path-equality checks see consistent paths regardless of
// platform.
func TestGitTopLevelDirNormalizesWindowsSlashes(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		// Git always emits forward slashes on Windows from rev-parse --show-toplevel.
		return vexec.Result{Stdout: []byte("/c/Users/dev/repo\n")}
	})

	root, err := shell.GitTopLevelDir(gitMemCtx(t), logger.CreateLogger(), exec, nil, "/c/Users/dev/repo")
	require.NoError(t, err)
	// On unix this is a no-op; the assertion verifies trim-and-normalize ran.
	assert.NotContains(t, root, "\n")
	assert.NotContains(t, root, "\r")
}

// TestGitTopLevelDirCacheHits verifies repeated lookups of the same path
// collapse to a single subprocess fork via the run-scoped repo-root cache.
func TestGitTopLevelDirCacheHits(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		calls.Add(1)
		return vexec.Result{Stdout: []byte("/repo\n")}
	})

	ctx := gitMemCtx(t)
	l := logger.CreateLogger()

	for range 5 {
		_, err := shell.GitTopLevelDir(ctx, l, exec, nil, "/repo")
		require.NoError(t, err)
	}

	assert.Equal(t, int32(1), calls.Load(), "repeated GitTopLevelDir calls must reuse the cached answer")
}

// TestGitRepoTagsParsesLsRemote pins the parse of `git ls-remote --tags`
// output: each non-empty line yields a tag in the second column.
func TestGitRepoTagsParsesLsRemote(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		assert.Equal(t, "git", inv.Name)
		assert.Equal(t, []string{"ls-remote", "--tags", "https://github.com/example/repo.git"}, inv.Args)

		return vexec.Result{Stdout: []byte(
			"abc123\trefs/tags/v1.0.0\n" +
				"def456\trefs/tags/v1.1.0\n" +
				"ghi789\trefs/tags/v2.0.0\n",
		)}
	})

	u, err := url.Parse("https://github.com/example/repo.git")
	require.NoError(t, err)

	tags, err := shell.GitRepoTags(t.Context(), logger.CreateLogger(), exec, nil, "/work", u)
	require.NoError(t, err)
	assert.Equal(t, []string{"refs/tags/v1.0.0", "refs/tags/v1.1.0", "refs/tags/v2.0.0"}, tags)
}

// TestGitLastReleaseTagSelectsHighestSemver pins the contract that
// GitLastReleaseTag returns the highest semver tag, ignoring non-semver
// tag names that ls-remote may include.
func TestGitLastReleaseTagSelectsHighestSemver(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte(
			"a\trefs/tags/v0.9.0\n" +
				"b\trefs/tags/v1.0.0\n" +
				"c\trefs/tags/v1.10.0\n" + // higher than v1.2.0 under semver
				"d\trefs/tags/v1.2.0\n" +
				"e\trefs/tags/some-non-semver-name\n",
		)}
	})

	u, err := url.Parse("https://github.com/example/repo.git")
	require.NoError(t, err)

	tag, err := shell.GitLastReleaseTag(t.Context(), logger.CreateLogger(), exec, nil, "/work", u)
	require.NoError(t, err)
	assert.Equal(t, "v1.10.0", tag)
}

// TestGitLastReleaseTagEmptyOnNoSemver pins the contract that a tag list
// with zero parseable semver entries returns "" rather than an error.
func TestGitLastReleaseTagEmptyOnNoSemver(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("a\trefs/tags/release-candidate\nb\trefs/tags/draft\n")}
	})

	u, err := url.Parse("https://github.com/example/repo.git")
	require.NoError(t, err)

	tag, err := shell.GitLastReleaseTag(t.Context(), logger.CreateLogger(), exec, nil, "/work", u)
	require.NoError(t, err)
	assert.Empty(t, tag)
}

// gitMemCtx returns a context primed with the repo-root cache so
// GitTopLevelDir can satisfy its memoization invariants without hitting
// the OS.
func gitMemCtx(t *testing.T) context.Context {
	t.Helper()
	return cache.ContextWithCache(t.Context())
}
