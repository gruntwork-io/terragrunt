package shell_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitRepoTagsParsesLsRemote pins the parse of `git ls-remote --tags`
// output: each non-empty line yields a tag in the second column.
func TestGitRepoTagsParsesLsRemote(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		assert.Equal(t, "git", inv.Name)
		assert.Equal(
			t,
			[]string{"ls-remote", "--tags", "https://github.com/example/repo.git"},
			inv.Args,
		)

		return vexec.Result{Stdout: []byte(
			"abc123\trefs/tags/v1.0.0\n" +
				"def456\trefs/tags/v1.1.0\n" +
				"ghi789\trefs/tags/v2.0.0\n",
		)}
	})

	u, err := url.Parse("https://github.com/example/repo.git")
	require.NoError(t, err)

	v := venvtest.New().WithExec(exec)

	tags, err := shell.GitRepoTags(t.Context(), logger.CreateLogger(), v, "/work", u)
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

	v := venvtest.New().WithExec(exec)

	tag, err := shell.GitLastReleaseTag(t.Context(), logger.CreateLogger(), v, "/work", u)
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

	v := venvtest.New().WithExec(exec)

	tag, err := shell.GitLastReleaseTag(t.Context(), logger.CreateLogger(), v, "/work", u)
	require.NoError(t, err)
	assert.Empty(t, tag)
}
