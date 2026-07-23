package helpers_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGitServer(t *testing.T) {
	t.Parallel()

	s := helpers.NewGitServer(t)
	s.AddFixtures("test/fixtures/download/hello-world")

	require.NotEmpty(t, s.URL)
	require.True(t, strings.HasPrefix(s.URL, "http://"), "URL must be http: %q", s.URL)

	out, err := exec.CommandContext(t.Context(), "git", "ls-remote", s.URL).CombinedOutput()
	require.NoError(t, err, "git ls-remote: %s", out)

	refs := string(out)
	assert.Contains(t, refs, "refs/heads/main")

	for _, tag := range helpers.TerragruntMirrorTags {
		assert.Contains(t, refs, "refs/tags/"+tag, "tag %s missing from refs:\n%s", tag, refs)
	}

	for _, branch := range helpers.TerragruntMirrorBranches {
		assert.Contains(
			t,
			refs,
			"refs/heads/"+branch,
			"branch %s missing from refs:\n%s",
			branch,
			refs,
		)
	}
}

func TestGitServerCloneSubpath(t *testing.T) {
	t.Parallel()

	s := helpers.NewGitServer(t)
	s.AddFixtures("test/fixtures/download/hello-world")

	dst := t.TempDir()

	out, err := exec.CommandContext(t.Context(), "git", "clone", "--single-branch", "--branch=v0.93.2", s.URL, dst).
		CombinedOutput()
	require.NoError(t, err, "git clone: %s", out)

	helloWorldMain := filepath.Join(dst, "test", "fixtures", "download", "hello-world", "main.tf")
	require.FileExists(t, helloWorldMain)

	contents, err := os.ReadFile(helloWorldMain)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "git::"+s.URL,
		"hello-world/main.tf should have __MIRROR_URL__ substituted to the live server URL "+
			"so a clone of one fixture stays inside the server instead of resolving back to GitHub")
	assert.NotContains(t, string(contents), "__MIRROR_URL__")
	assert.NotContains(t, string(contents), "github.com/gruntwork-io/terragrunt.git")
}

func TestGitServerCommitsOnlyRequestedFixtures(t *testing.T) {
	t.Parallel()

	s := helpers.NewGitServer(t)
	s.AddFixtures("test/fixtures/download/hello-world")

	dst := t.TempDir()

	out, err := exec.CommandContext(t.Context(), "git", "clone", "--single-branch", "--branch=main", s.URL, dst).
		CombinedOutput()
	require.NoError(t, err, "git clone: %s", out)

	// hello-world is requested; hello-world-no-remote comes along as a
	// transitive reference. An unrelated fixture must not be served.
	assert.DirExists(t, filepath.Join(dst, "test", "fixtures", "download", "hello-world-no-remote"))
	assert.NoDirExists(t, filepath.Join(dst, "test", "fixtures", "stacks"))
}

func TestGitServerSourceURL(t *testing.T) {
	t.Parallel()

	s := helpers.NewGitServer(t)

	got := s.SourceURL("/test/fixtures/download/hello-world", "v0.93.2")
	want := "git::" + s.URL + "//test/fixtures/download/hello-world?ref=v0.93.2"
	assert.Equal(t, want, got)

	got = s.SourceURL("test/fixtures/inputs", "")
	want = "git::" + s.URL + "//test/fixtures/inputs"
	assert.Equal(t, want, got)
}

//nolint:paralleltest // RequireSSH calls t.Setenv, which panics in parallel tests.
func TestGitServerSSHClone(t *testing.T) {
	s := helpers.NewGitServer(t)
	s.AddFixtures("test/fixtures/download/hello-world")
	s.RequireSSH()

	require.NotEmpty(t, s.SSHURL)
	require.True(t, strings.HasPrefix(s.SSHURL, "ssh://git@127.0.0.1:"), "SSH URL: %q", s.SSHURL)

	out, err := exec.CommandContext(t.Context(), "git", "ls-remote", s.SSHURL).CombinedOutput()
	require.NoError(t, err, "git ls-remote over SSH: %s", out)

	refs := string(out)
	assert.Contains(t, refs, "refs/heads/main")
	assert.Contains(t, refs, "refs/tags/v0.93.2")
}
