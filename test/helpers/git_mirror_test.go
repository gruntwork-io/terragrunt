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

func TestStartTerragruntMirror(t *testing.T) {
	t.Parallel()

	m := helpers.StartTerragruntMirror(t)

	require.NotEmpty(t, m.URL)
	require.NotEmpty(t, m.HeadSHA)
	require.True(t, strings.HasPrefix(m.URL, "http://"), "URL must be http: %q", m.URL)

	out, err := exec.CommandContext(t.Context(), "git", "ls-remote", m.URL).CombinedOutput()
	require.NoError(t, err, "git ls-remote: %s", out)

	refs := string(out)
	assert.Contains(t, refs, "refs/heads/main")

	for _, tag := range []string{"v0.53.8", "v0.67.4", "v0.83.2", "v0.93.2", "v0.99.1"} {
		assert.Contains(t, refs, "refs/tags/"+tag, "tag %s missing from refs:\n%s", tag, refs)
	}

	assert.Contains(t, refs, m.HeadSHA, "HEAD SHA %s missing from ls-remote output:\n%s", m.HeadSHA, refs)
}

func TestTerragruntMirrorCloneSubpath(t *testing.T) {
	t.Parallel()

	m := helpers.StartTerragruntMirror(t)

	dst := t.TempDir()

	out, err := exec.CommandContext(t.Context(), "git", "clone", "--single-branch", "--branch=v0.93.2", m.URL, dst).CombinedOutput()
	require.NoError(t, err, "git clone: %s", out)

	helloWorldMain := filepath.Join(dst, "test", "fixtures", "download", "hello-world", "main.tf")
	require.FileExists(t, helloWorldMain)

	contents, err := os.ReadFile(helloWorldMain)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "git::"+m.URL,
		"hello-world/main.tf should have __MIRROR_URL__ substituted to the live mirror URL "+
			"so a clone of one fixture stays inside the mirror instead of resolving back to GitHub")
	assert.NotContains(t, string(contents), "__MIRROR_URL__")
	assert.NotContains(t, string(contents), "github.com/gruntwork-io/terragrunt.git")
}

func TestTerragruntMirrorSourceURL(t *testing.T) {
	t.Parallel()

	m := helpers.StartTerragruntMirror(t)

	got := m.SourceURL("/test/fixtures/download/hello-world", "v0.93.2")
	want := "git::" + m.URL + "//test/fixtures/download/hello-world?ref=v0.93.2"
	assert.Equal(t, want, got)

	got = m.SourceURL("test/fixtures/inputs", "")
	want = "git::" + m.URL + "//test/fixtures/inputs"
	assert.Equal(t, want, got)
}

//nolint:paralleltest // RequireSSH calls t.Setenv, which panics in parallel tests.
func TestTerragruntMirrorSSHClone(t *testing.T) {
	m := helpers.StartTerragruntMirror(t)
	m.RequireSSH(t)

	require.NotEmpty(t, m.SSHURL)
	require.True(t, strings.HasPrefix(m.SSHURL, "ssh://git@127.0.0.1:"), "SSH URL: %q", m.SSHURL)

	out, err := exec.CommandContext(t.Context(), "git", "ls-remote", m.SSHURL).CombinedOutput()
	require.NoError(t, err, "git ls-remote over SSH: %s", out)

	refs := string(out)
	assert.Contains(t, refs, "refs/heads/main")
	assert.Contains(t, refs, "refs/tags/v0.93.2")
}
