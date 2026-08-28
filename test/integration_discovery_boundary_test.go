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

func TestDiscoveryBoundaryBoundsGraphTraversal(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDiscoveryBoundary)
	prodDir := filepath.Join(tmpEnvPath, testFixtureDiscoveryBoundary, "prod")

	testCases := []struct {
		name     string
		args     string
		expected []string
	}{
		{
			name:     "dependency traversal leaves the working directory when unbounded",
			args:     "--filter '{./app}...'",
			expected: []string{"app", "db", "../shared/dns"},
		},
		{
			name:     "dependency traversal stops at the boundary",
			args:     "--filter '{./app}...' --discovery-boundary .",
			expected: []string{"app", "db"},
		},
		{
			name:     "dependent traversal keeps what the boundary encloses",
			args:     "--filter '...{./db}' --discovery-boundary .",
			expected: []string{"app", "db"},
		},
		{
			name:     "inline operand reaches wider than the boundary it overrides",
			args:     "--filter '{./app}...(..)' --discovery-boundary .",
			expected: []string{"app", "db", "../shared/dns"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := "terragrunt find --experiment bounded-discovery --no-color --working-dir " + prodDir + " " + tc.args

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			assert.ElementsMatch(t, tc.expected, discoveredPaths(stdout))
		})
	}
}

// Test that the edge to a dependency the boundary withholds survives. The unit
// that stays has to be ordered against that dependency and has to read its
// outputs, so the boundary drops the component without dropping the edge.
func TestDiscoveryBoundaryKeepsEdgesToWithheldDependencies(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDiscoveryBoundary)
	prodDir := filepath.Join(tmpEnvPath, testFixtureDiscoveryBoundary, "prod")

	cmd := "terragrunt dag graph --experiment bounded-discovery --no-color --working-dir " +
		prodDir + " --discovery-boundary ."

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Contains(t, stdout, `"app" -> "../shared/dns"`)
	assert.Contains(t, stdout, `"app" -> "db"`)

	// A node line carries no arrow, so its absence is what says shared/dns was
	// withheld rather than merely pointed at.
	assert.NotContains(t, stdout, `"../shared/dns" ;`)
}

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

// Test that a boundary excluding the working directory is refused for filters
// that traverse dependents, which search upward from the working directory and
// could never reach anything within such a boundary.
func TestDiscoveryBoundaryRejectsDependentTraversalOutsideIt(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDiscoveryBoundary)
	fixtureDir := filepath.Join(tmpEnvPath, testFixtureDiscoveryBoundary)

	cmd := "terragrunt find --experiment bounded-discovery --no-color --working-dir " +
		fixtureDir + " --filter '...{./prod/db}' --discovery-boundary ./prod"

	// The typed error does not survive the process boundary, so the class of
	// failure is all a test at this level can pin.
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.ErrorContains(t, err, "discovery boundary")
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
