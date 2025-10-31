package test_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"
)

const (
	testFixtureFindBasic                = "fixtures/find/basic"
	testFixtureFindHidden               = "fixtures/find/hidden"
	testFixtureFindDAG                  = "fixtures/find/dag"
	testFixtureFindInternalVExternal    = "fixtures/find/internal-v-external"
	testFixtureFindExclude              = "fixtures/exclude/basic"
	testFixtureFindInclude              = "fixtures/find/include"
	testFixtureFindReadTerragruntConfig = "fixtures/find/read-terragrunt-config"
)

func TestFindBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindBasic)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --no-color --working-dir "+testFixtureFindBasic)
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.Equal(t, "stack\nunit\n", stdout)
}

func TestFindBasicJSON(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindBasic)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --no-color --working-dir "+testFixtureFindBasic+" --json")
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.JSONEq(t, `[{"type": "stack", "path": "stack"}, {"type": "unit", "path": "unit"}]`, stdout)
}

func TestFindHidden(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		expected string
		hidden   bool
	}{
		{
			name:     "visible",
			expected: "stack\nunit\n",
		},
		{
			name:     "hidden",
			hidden:   true,
			expected: ".hide/unit\nstack\nunit\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFindHidden)

			cmd := "terragrunt find --no-color --working-dir " + testFixtureFindHidden

			if tc.hidden {
				cmd += " --hidden"
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			assert.Empty(t, stderr)
			// Normalize path separators in the output for cross-platform compatibility
			normalizedStdout := filepath.ToSlash(stdout)
			assert.Equal(t, tc.expected, normalizedStdout)
		})
	}
}

func TestFindDAG(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		sort     string
		expected string
	}{
		{name: "alpha", sort: "alpha", expected: "a-dependent\nb-dependency\nc-mixed-deps\nd-dependencies-only\n"},
		{name: "dag", sort: "dag", expected: "b-dependency\na-dependent\nd-dependencies-only\nc-mixed-deps\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFindDAG)

			cmd := "terragrunt find --no-color --working-dir " + testFixtureFindDAG

			if tc.sort == "dag" {
				cmd += " --dag"
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			assert.Empty(t, stderr)
			assert.Equal(t, tc.expected, stdout)
		})
	}
}

func TestFindDAGWithMixedDependencies(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindDAG)

	testCases := []struct {
		name     string
		args     string
		expected string
	}{
		{
			name:     "dag with dependencies output",
			args:     "--dag --dependencies",
			expected: "b-dependency\na-dependent\nd-dependencies-only\nc-mixed-deps\n",
		},
		{
			name:     "dag with dependencies json output",
			args:     "--dag --dependencies --json",
			expected: `[{"type":"unit","path":"b-dependency"},{"type":"unit","path":"a-dependent","dependencies":["b-dependency"]},{"type":"unit","path":"d-dependencies-only","dependencies":["a-dependent"]},{"type":"unit","path":"c-mixed-deps","dependencies":["a-dependent","d-dependencies-only"]}]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFindDAG)

			cmd := "terragrunt find --no-color --working-dir " + testFixtureFindDAG + " " + tc.args

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			assert.Empty(t, stderr)

			if strings.Contains(tc.args, "--json") {
				jsonStringsEqual(t, tc.expected, stdout)
			} else {
				assert.Equal(t, tc.expected, stdout)
			}
		})
	}
}

// jsonStringsEqual compares two JSON strings for equivalence, ignoring the order of nested arrays.
func jsonStringsEqual(t *testing.T, expected, actual string, msgAndArgs ...any) bool {
	t.Helper()

	patch, err := jsondiff.CompareJSON([]byte(expected), []byte(actual), jsondiff.Equivalent())
	require.NoErrorf(t, err, fmt.Sprintf("Error comparing JSON strings: %v", err), msgAndArgs...)
	require.Emptyf(t, patch, fmt.Sprintf("JSON strings are not equal\nExpected: %s\nActual: %s", expected, actual), msgAndArgs...)

	return true
}

func TestFindExternalDependencies(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip(`This functionality will break once the filter flag experiment is generally available.
We don't automatically discover external dependencies when going through discovery via the filter flag.`)
	}

	helpers.CleanupTerraformFolder(t, testFixtureFindInternalVExternal)

	internalDir := filepath.Join(testFixtureFindInternalVExternal, "internal")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --no-color --working-dir "+internalDir+" --dependencies --external")
	require.NoError(t, err)

	assert.Empty(t, stderr)
	// Normalize path separators in the output for cross-platform compatibility
	normalizedStdout := filepath.ToSlash(stdout)
	assert.Equal(t, "../external/c-dependency\na-dependent\nb-dependency\n", normalizedStdout)

	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --no-color --working-dir "+internalDir+" --dependencies")
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.Equal(t, "a-dependent\nb-dependency\n", stdout)
}

func TestFindExternalDependenciesWithFilterFlag(t *testing.T) {
	t.Parallel()

	if !helpers.IsExperimentMode(t) {
		t.Skip("This only works when the filter flag experiment is enabled until it is generally available.")
	}

	helpers.CleanupTerraformFolder(t, testFixtureFindInternalVExternal)

	internalDir := filepath.Join(testFixtureFindInternalVExternal, "internal")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt find --no-color --working-dir "+internalDir+" --dependencies --external --filter '{./**}...'",
	)
	require.NoError(t, err)

	assert.Empty(t, stderr)
	// Normalize path separators in the output for cross-platform compatibility
	normalizedStdout := filepath.ToSlash(stdout)
	assert.Equal(t, "../external/c-dependency\na-dependent\nb-dependency\n", normalizedStdout)

	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --no-color --working-dir "+internalDir+" --dependencies")
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.Equal(t, "a-dependent\nb-dependency\n", stdout)
}

func TestFindInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindInclude)

	workdir := testFixtureFindInclude

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --no-color --working-dir "+workdir+" --include --json")
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.JSONEq(t, `[{"type":"unit","path":"bar","include":{"cloud":"cloud.hcl"}},{"type":"unit","path":"foo"}]`, stdout)
}

func TestFindExclude(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		args           string
		expectedOutput string
		expectedPaths  []string
	}{
		{
			name:          "show exclude configs",
			args:          "--exclude",
			expectedPaths: []string{"unit1", "unit2", "unit3"},
		},
		{
			name:          "exclude plan command",
			args:          "--queue-construct-as plan",
			expectedPaths: []string{"unit2", "unit3"},
		},
		{
			name:          "exclude apply command",
			args:          "--queue-construct-as apply",
			expectedPaths: []string{"unit1", "unit3"},
		},
		{
			name:          "show exclude configs with json",
			args:          "--exclude --json",
			expectedPaths: []string{"unit1", "unit2", "unit3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFindExclude)

			cmd := fmt.Sprintf("terragrunt find --no-color --working-dir %s %s", testFixtureFindExclude, tc.args)
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)
			assert.Empty(t, stderr)

			if strings.Contains(tc.args, "--json") {
				var configs find.FoundComponents

				err = json.Unmarshal([]byte(stdout), &configs)
				require.NoError(t, err)

				var paths []string
				for _, config := range configs {
					paths = append(paths, config.Path)
					if strings.Contains(tc.args, "--exclude") {
						switch config.Path {
						case "unit1":
							assert.NotNil(t, config.Exclude)
							assert.Contains(t, config.Exclude.Actions, "plan")
						case "unit2":
							assert.NotNil(t, config.Exclude)
							assert.Contains(t, config.Exclude.Actions, "apply")
						default:
							assert.Nil(t, config.Exclude)
						}
					}
				}

				assert.ElementsMatch(t, tc.expectedPaths, paths)
			} else {
				paths := strings.Fields(stdout)
				assert.ElementsMatch(t, tc.expectedPaths, paths)
			}
		})
	}
}

func TestFindQueueConstructAs(t *testing.T) {
	t.Parallel()

	// I'm using the list fixture here because it's more convenient.
	testFixtureQueueConstruct := "fixtures/list/dag"
	helpers.CleanupTerraformFolder(t, testFixtureQueueConstruct)

	testCases := []struct {
		name           string
		args           string
		expectedOutput string
		expectedPaths  []string
	}{
		{
			name: "up command",
			args: "--queue-construct-as plan",
			expectedPaths: []string{
				"stacks/live/dev",
				"stacks/live/prod",
				"units/live/dev/vpc",
				"units/live/prod/vpc",
				"units/live/dev/db",
				"units/live/prod/db",
				"units/live/dev/ec2",
				"units/live/prod/ec2",
			},
		},
		{
			name: "down command",
			args: "--queue-construct-as destroy",
			expectedPaths: []string{
				"stacks/live/dev",
				"stacks/live/prod",
				"units/live/dev/ec2",
				"units/live/prod/ec2",
				"units/live/dev/db",
				"units/live/prod/db",
				"units/live/dev/vpc",
				"units/live/prod/vpc",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureQueueConstruct)

			cmd := fmt.Sprintf("terragrunt find --json --no-color --working-dir %s %s", testFixtureQueueConstruct, tc.args)
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)
			assert.Empty(t, stderr)

			var configs find.FoundComponents

			err = json.Unmarshal([]byte(stdout), &configs)
			require.NoError(t, err)

			var paths []string
			for _, config := range configs {
				// Normalize path separators for cross-platform compatibility
				paths = append(paths, filepath.ToSlash(config.Path))
			}

			assert.Equal(t, tc.expectedPaths, paths)
		})
	}
}

// TestFindWithReadTerragruntConfig tests that the find command works correctly
// when using read_terragrunt_config with dependencies.
func TestFindWithReadTerragruntConfig(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindReadTerragruntConfig)

	testCases := []struct {
		name     string
		args     string
		expected string
	}{
		{
			name:     "find with dag and json",
			args:     "--dag --json",
			expected: `[{"type":"unit","path":"module"},{"type":"unit","path":"."}]`,
		},
		{
			name:     "find with json",
			args:     "--json",
			expected: `[{"type":"unit","path":"."},{"type":"unit","path":"module"}]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFindReadTerragruntConfig)

			cmd := fmt.Sprintf("terragrunt find --no-color --working-dir %s %s", testFixtureFindReadTerragruntConfig, tc.args)
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)

			// The command should succeed without errors
			require.NoError(t, err, "find command should not fail")

			// There should be no error output
			assert.Empty(t, stderr, "stderr should be empty - no parse errors should occur")

			// Verify the JSON output matches expected
			jsonStringsEqual(t, tc.expected, stdout)
		})
	}
}
