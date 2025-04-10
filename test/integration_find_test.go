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
)

const (
	testFixtureFindBasic             = "fixtures/find/basic"
	testFixtureFindHidden            = "fixtures/find/hidden"
	testFixtureFindDAG               = "fixtures/find/dag"
	testFixtureFindInternalVExternal = "fixtures/find/internal-v-external"
	testFixtureFindExclude           = "fixtures/exclude/basic"
)

func TestFindBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindBasic)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --experiment cli-redesign --no-color --working-dir "+testFixtureFindBasic)
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.Equal(t, "stack\nunit\n", stdout)
}

func TestFindBasicJSON(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindBasic)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --experiment cli-redesign --no-color --working-dir "+testFixtureFindBasic+" --json")
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

			cmd := "terragrunt find --experiment cli-redesign --no-color --working-dir " + testFixtureFindHidden

			if tc.hidden {
				cmd += " --hidden"
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			assert.Empty(t, stderr)
			assert.Equal(t, tc.expected, stdout)
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
		{name: "alpha", sort: "alpha", expected: "a-dependent\nb-dependency\n"},
		{name: "dag", sort: "dag", expected: "b-dependency\na-dependent\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFindDAG)

			cmd := "terragrunt find --experiment cli-redesign --no-color --working-dir " + testFixtureFindDAG

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

func TestFindExternalDependencies(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindInternalVExternal)

	internalDir := filepath.Join(testFixtureFindInternalVExternal, "internal")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --experiment cli-redesign --no-color --working-dir "+internalDir+" --dependencies --external")
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.Equal(t, "../external/c-dependency\na-dependent\nb-dependency\n", stdout)

	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --experiment cli-redesign --no-color --working-dir "+internalDir+" --dependencies")
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.Equal(t, "a-dependent\nb-dependency\n", stdout)
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

			cmd := fmt.Sprintf("terragrunt find --experiment cli-redesign --no-color --working-dir %s %s", testFixtureFindExclude, tc.args)
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)
			assert.Empty(t, stderr)

			if strings.Contains(tc.args, "--json") {
				var configs find.FoundConfigs
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
