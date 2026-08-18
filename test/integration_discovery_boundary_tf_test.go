//go:build tf

package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test which units a bounded run applies. Dependency traversal follows app's
// declared path out of prod, so the boundary is what decides whether the run
// reaches shared/dns or stops at the edge of prod.
func TestTFDiscoveryBoundaryBoundsWhatRunAllApplies(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		args     string
		expected []string
	}{
		{
			name:     "unbounded traversal applies the unit outside the working directory",
			args:     "--filter '{./app}...'",
			expected: []string{"app", "db", "shared/dns"},
		},
		{
			name:     "the boundary withholds it",
			args:     "--filter '{./app}...' --discovery-boundary .",
			expected: []string{"app", "db"},
		},
		{
			name:     "an inline operand reaching wider overrides the boundary",
			args:     "--filter '{./app}...(..)' --discovery-boundary .",
			expected: []string{"app", "db", "shared/dns"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixtureDir, prodDir := copyDiscoveryBoundaryFixture(t)
			reportFile := filepath.Join(prodDir, "report.json")

			_, _, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt run --all --non-interactive --experiment bounded-discovery --working-dir "+prodDir+
					" "+tc.args+" --report-file "+reportFile+" --report-format json -- apply -auto-approve",
			)
			require.NoError(t, err)

			require.FileExists(t, reportFile)

			runs, parseErr := report.ParseJSONRunsFromFile(reportFile)
			require.NoError(t, parseErr)

			assert.ElementsMatch(t, tc.expected, relativeRunNames(t, runs.Names(), fixtureDir))
		})
	}
}

// Test that the units a bounded run does apply are still ordered against the
// dependency it withholds and still read that dependency's outputs. Withholding
// a unit from the run is not the same as pretending it is not there.
func TestTFDiscoveryBoundaryRunAllConsumesWithheldDependency(t *testing.T) {
	t.Parallel()

	fixtureDir, prodDir := copyDiscoveryBoundaryFixture(t)

	// Apply the out-of-boundary dependency on its own, so its outputs come from
	// state rather than from the mock prod/app declares.
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+filepath.Join(fixtureDir, "shared", "dns"),
	)
	require.NoError(t, err)

	reportFile := filepath.Join(prodDir, "report.json")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --experiment bounded-discovery --working-dir "+prodDir+
			" --filter '{./app}...' --discovery-boundary . --report-file "+reportFile+
			" --report-format json -- apply -auto-approve",
	)
	require.NoError(t, err)

	require.FileExists(t, reportFile)

	runs, parseErr := report.ParseJSONRunsFromFile(reportFile)
	require.NoError(t, parseErr)

	require.ElementsMatch(t, []string{"app", "db"}, relativeRunNames(t, runs.Names(), fixtureDir))

	app := runs.FindByName("app")
	require.NotNil(t, app)

	db := runs.FindByName("db")
	require.NotNil(t, db)

	assert.True(t, db.Started.Before(app.Started), "the in-boundary dependency still runs first")

	// dns-applied is what shared/dns puts in state; dns-mocked is what app falls
	// back to when the dependency cannot be read.
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -raw app_id -no-color --non-interactive --working-dir "+filepath.Join(prodDir, "app"),
	)
	require.NoError(t, err)

	assert.Contains(t, stdout, "dns-applied", "the withheld dependency's outputs were read from its state")
}

// copyDiscoveryBoundaryFixture stages the fixture and returns its root and the
// prod directory the bounded runs work from.
func copyDiscoveryBoundaryFixture(t *testing.T) (string, string) {
	t.Helper()

	helpers.CleanupTerraformFolder(t, testFixtureDiscoveryBoundary)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDiscoveryBoundary)
	fixtureDir := filepath.Join(tmpEnvPath, testFixtureDiscoveryBoundary)

	return fixtureDir, filepath.Join(fixtureDir, "prod")
}

// relativeRunNames restates the reported unit names against the fixture root, so
// that a unit reported by absolute path because it sits outside the working
// directory reads the same way as one reported relative to it.
func relativeRunNames(t *testing.T, names []string, fixtureDir string) []string {
	t.Helper()

	relative := make([]string, 0, len(names))

	for _, name := range names {
		if filepath.IsAbs(name) {
			rel, err := filepath.Rel(fixtureDir, name)
			require.NoError(t, err)

			name = filepath.ToSlash(rel)
		}

		relative = append(relative, name)
	}

	return relative
}
