//go:build azure

package test_test

import (
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	liveExampleStartTimeout = 3 * time.Minute
	liveExamplePollInterval = 5 * time.Second
)

// cloneLiveExample clones a live example repository at ref, installs its pinned tools and returns its path.
func cloneLiveExample(t *testing.T, repo, ref string) string {
	t.Helper()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	repoDir := filepath.Join(tmpDir, "live-example")

	helpers.ExecWithTestLogger(t, tmpDir, "git", "clone", repo, repoDir)
	helpers.ExecWithTestLogger(t, repoDir, "git", "checkout", ref)
	helpers.ExecWithTestLogger(t, repoDir, "mise", "trust")
	helpers.ExecWithTestLogger(t, repoDir, "mise", "install")

	return repoDir
}

// liveExampleTerragrunt returns the arguments of an `env` command that runs the example's pinned terragrunt.
func liveExampleTerragrunt(env []string, args ...string) []string {
	return slices.Concat(env, []string{"mise", "x", "terragrunt", "--", "terragrunt"}, args)
}

// requireHelloWorld polls the HTTPS url until it answers 200 with a body that contains want.
func requireHelloWorld(t *testing.T, url, want string) {
	t.Helper()

	require.True(t, strings.HasPrefix(url, "https://"), "expected an HTTPS URL, got %q", url)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		require.NoError(c, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(c, err)

		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(c, err)

		assert.Equal(c, http.StatusOK, resp.StatusCode)
		assert.Contains(c, string(body), want)
	}, liveExampleStartTimeout, liveExamplePollInterval)
}
