package test_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixture holds a dependency that crosses out of prod:
//
//	prod/app     depends on prod/db and shared/dns
//	prod/db
//	shared/dns
const testFixtureDiscoveryBoundary = "fixtures/discovery-boundary"

// Test that a working directory reached through a symlink still compares as
// inside its own boundary. Terragrunt resolves the boundary through the
// filesystem while the walk keeps the path it was handed, and a component named
// in one spelling and bounded in the other is a component silently dropped.
func TestDiscoveryBoundaryUnderSymlinkedWorkingDir(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("creating symlinks on Windows requires elevated privileges")
	}

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDiscoveryBoundary)
	fixtureDir := filepath.Join(tmpEnvPath, testFixtureDiscoveryBoundary)

	linkDir := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(fixtureDir, linkDir))

	cmd := "terragrunt find --experiment bounded-discovery --no-color --working-dir " +
		filepath.Join(linkDir, "prod") + " --filter '...{./db}' --discovery-boundary ."

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"app", "db"}, discoveredPaths(stdout))
}

// discoveredPaths returns the unit paths a discovery command printed.
func discoveredPaths(stdout string) []string {
	var paths []string

	for line := range strings.SplitSeq(stdout, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}

	return paths
}
