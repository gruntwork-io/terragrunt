package helpers_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:paralleltest // RequireSSH puts the key in the process environment, where the git subprocess below reads it.
func TestGitServerRequireSSHBeforeFixtures(t *testing.T) {
	s := helpers.NewGitServer(t)
	s.RequireSSH()

	sshURL := s.SSHURL
	require.NotEmpty(t, sshURL)

	// A second call reuses the running endpoint.
	s.RequireSSH()

	assert.Equal(t, sshURL, s.SSHURL)

	// Fixtures added once SSH is live reach the SSH mirror too.
	s.AddFixtures("test/fixtures/download/hello-world")

	dst := t.TempDir()

	out, err := exec.CommandContext(t.Context(), "git", "clone", "--branch=main", s.SSHURL, dst).CombinedOutput()
	require.NoError(t, err, "git clone over SSH: %s", out)

	contents, err := os.ReadFile(filepath.Join(dst, "test", "fixtures", "download", "hello-world", "main.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(contents), "git::"+s.URL+"//test/fixtures/download/hello-world-no-remote")
	assert.NotContains(t, string(contents), helpers.MirrorURLPlaceholder)
}
