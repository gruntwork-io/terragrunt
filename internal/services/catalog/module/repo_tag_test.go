package module_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLatestTag_FindsHighestSemver(t *testing.T) {
	t.Parallel()

	exec := memGitExecForTags(t, []string{"v1.0.0", "v1.10.2", "v1.5.0", "not-semver"})
	l := logger.CreateLogger()

	repo := &module.Repo{RemoteURL: "https://example.com/org/repo.git"}

	repo.ResolveLatestTag(t.Context(), l, exec)

	assert.Equal(t, "v1.10.2", repo.LatestTag)
}

func TestResolveLatestTag_NoTags(t *testing.T) {
	t.Parallel()

	// A `git ls-remote` with no matching refs prints nothing and exits
	// successfully; the GitRunner surfaces ErrNoMatchingReference which
	// ResolveLatestTag swallows, leaving LatestTag empty.
	exec := memGitExecForTags(t, nil)
	l := logger.CreateLogger()

	repo := &module.Repo{RemoteURL: "https://example.com/org/repo.git"}

	repo.ResolveLatestTag(t.Context(), l, exec)

	assert.Empty(t, repo.LatestTag)
}

func TestResolveLatestTag_EmptyRemoteURL(t *testing.T) {
	t.Parallel()

	// No remote means no git fork; pass a handler that fails if reached.
	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		assert.Fail(t, "ResolveLatestTag must not dispatch git when RemoteURL is empty")
		return vexec.Result{}
	})

	l := logger.CreateLogger()

	repo := &module.Repo{}

	repo.ResolveLatestTag(t.Context(), l, exec)

	assert.Empty(t, repo.LatestTag)
}

// TestResolveLatestTag_SkipsPrereleases pins the contract that prerelease
// tags (e.g. v2.0.0-rc1) are not considered "latest" releases. The
// original integration-style test couldn't isolate this branch cleanly;
// the mem-exec version makes the prerelease filter directly observable.
func TestResolveLatestTag_SkipsPrereleases(t *testing.T) {
	t.Parallel()

	exec := memGitExecForTags(t, []string{"v1.0.0", "v2.0.0-rc1", "v1.5.0"})
	l := logger.CreateLogger()

	repo := &module.Repo{RemoteURL: "https://example.com/org/repo.git"}

	repo.ResolveLatestTag(t.Context(), l, exec)

	assert.Equal(t, "v1.5.0", repo.LatestTag, "v2.0.0-rc1 is a prerelease and must be skipped")
}

// TestResolveLatestTag_GitFailureLeavesTagEmpty pins the contract that
// a failing git ls-remote (e.g. unreachable remote) does not propagate
// as an error; ResolveLatestTag swallows it and leaves LatestTag empty
// so discovery can continue.
func TestResolveLatestTag_GitFailureLeavesTagEmpty(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 128, Stderr: []byte("fatal: unable to access remote\n")}
	})

	l := logger.CreateLogger()

	repo := &module.Repo{RemoteURL: "https://unreachable.example/repo.git"}

	require.NotPanics(t, func() {
		repo.ResolveLatestTag(t.Context(), l, exec)
	})

	assert.Empty(t, repo.LatestTag, "a failed ls-remote must leave LatestTag empty without panicking")
}

// memGitExecForTags returns a vexec.Exec whose `git ls-remote` response is
// the supplied tags. Any other invocation fails the test so a regression
// that fires unexpected git subcommands is caught here.
func memGitExecForTags(t *testing.T, tags []string) vexec.Exec {
	t.Helper()

	return vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name != "git" || len(inv.Args) == 0 || inv.Args[0] != "ls-remote" {
			assert.Fail(t, "unexpected git invocation", "name=%q args=%v", inv.Name, inv.Args)
			return vexec.Result{ExitCode: 1}
		}

		var stdout []byte
		for i, tag := range tags {
			stdout = append(stdout, []byte(makeLsRemoteLine(i, tag))...)
		}

		return vexec.Result{Stdout: stdout}
	})
}

// makeLsRemoteLine renders one line of `git ls-remote --tags` output:
// "<40-char-hash>\trefs/tags/<tag>\n".
func makeLsRemoteLine(seed int, tag string) string {
	hash := "0000000000000000000000000000000000000000"
	hash = hash[:38] + string("0123456789abcdef"[seed%16]) + string("0123456789abcdef"[(seed+1)%16])

	return hash + "\trefs/tags/" + tag + "\n"
}
