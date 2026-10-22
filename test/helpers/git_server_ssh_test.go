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

func TestGitServerRequireSSHSkipsWithoutTools(t *testing.T) {
	testCases := []struct {
		name         string
		fakeBinaries []string
	}{
		{
			name: "no ssh client",
		},
		{
			name:         "no git-upload-pack",
			fakeBinaries: []string{"ssh"},
		},
	}

	for _, tc := range testCases {
		var (
			skippedTest *testing.T
			reached     bool
		)

		t.Run(tc.name, func(t *testing.T) {
			skippedTest = t

			s := helpers.NewGitServer(t)

			binDir := t.TempDir()
			for _, name := range tc.fakeBinaries {
				thTWriteFakeBinary(t, binDir, name)
			}

			t.Setenv("PATH", binDir)

			s.RequireSSH()

			reached = true
		})

		require.NotNil(t, skippedTest)
		assert.True(t, skippedTest.Skipped(), "RequireSSH should skip the test")
		assert.False(t, reached, "RequireSSH should stop the test")
	}
}

// thTWriteFakeBinary writes an executable stub named name into dir, plus a .exe copy for Windows.
func thTWriteFakeBinary(t *testing.T, dir, name string) {
	t.Helper()

	for _, file := range []string{name, name + ".exe"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte("#!/bin/sh\nexit 1\n"), 0o755))
	}
}
