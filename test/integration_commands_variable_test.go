package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	testFixtureCommandsVariable    = "fixtures/commands-variable"
	commandsVariableExperimentFlag = " --experiment commands-variable"
)

func TestCommandsVariableInUnitIncludeAndRead(t *testing.T) {
	t.Parallel()

	root := copyCommandsVariableFixture(t)

	inputs := renderCommandsVariableInputs(t, filepath.Join(root, "live", "app"), commandsVariableExperimentFlag)

	assert.Equal(t, map[string]any{
		"vars":                 commandList(config.TerraformCommandsNeedVars),
		"locking":              commandList(config.TerraformCommandsNeedLocking),
		"input":                commandList(config.TerraformCommandsNeedInput),
		"parallelism":          commandList(config.TerraformCommandsNeedParallelism),
		"indexed":              commandList(config.TerraformCommandsNeedVars),
		"function_vars":        commandList(config.TerraformCommandsNeedVars),
		"function_locking":     commandList(config.TerraformCommandsNeedLocking),
		"function_input":       commandList(config.TerraformCommandsNeedInput),
		"function_parallelism": commandList(config.TerraformCommandsNeedParallelism),
		"env_input":            commandList(config.TerraformCommandsNeedInput),
		"root_locking":         commandList(config.TerraformCommandsNeedLocking),
		"plan_accepts_vars":    true,
		"not_defined":          "not-defined",
	}, inputs)
}

func TestCommandsVariableInRenderAll(t *testing.T) {
	t.Parallel()

	root := copyCommandsVariableFixture(t)
	workingDir := filepath.Join(root, "live")

	helpers.RunTerragrunt(
		t,
		"terragrunt render --all --format json --write --non-interactive --working-dir "+workingDir+
			commandsVariableExperimentFlag,
	)

	rendered, err := os.ReadFile(filepath.Join(workingDir, "json", "terragrunt.rendered.json"))
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"vars":    commandList(config.TerraformCommandsNeedVars),
		"indexed": commandList(config.TerraformCommandsNeedParallelism),
	}, commandsVariableInputs(t, rendered))
}

func TestCommandsVariableInGeneratedStack(t *testing.T) {
	t.Parallel()

	root := copyCommandsVariableFixture(t)
	stackDir := filepath.Join(root, "stack", "live")
	unitDir := filepath.Join(stackDir, config.StackDir, "app")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+stackDir+commandsVariableExperimentFlag)

	assert.Equal(t, map[string]any{
		"vars":        commandList(config.TerraformCommandsNeedVars),
		"locking":     commandList(config.TerraformCommandsNeedLocking),
		"input":       commandList(config.TerraformCommandsNeedInput),
		"parallelism": commandList(config.TerraformCommandsNeedParallelism),
	}, renderCommandsVariableInputs(t, unitDir, commandsVariableExperimentFlag))
}

func TestCommandsVariableInGeneratedAutoIncludeNeedsNoExperiment(t *testing.T) {
	t.Parallel()

	helpers.SkipInExperimentMode(t, experiment.CommandsVariable)

	root := copyCommandsVariableFixture(t)
	stackDir := filepath.Join(root, "autoinclude", "live")
	unitDir := filepath.Join(stackDir, config.StackDir, "app")

	helpers.RunTerragrunt(t, "terragrunt stack generate --working-dir "+stackDir+commandsVariableExperimentFlag)

	assert.Equal(t, map[string]any{
		"unit":      "app",
		"auto_vars": commandList(config.TerraformCommandsNeedVars),
	}, renderCommandsVariableInputs(t, unitDir, ""))
}

func TestCommandsVariableInStackDependencyDiscovery(t *testing.T) {
	t.Parallel()

	root := copyCommandsVariableFixture(t)
	workingDir := filepath.Join(root, "discovery")
	unitName := config.TerraformCommandsNeedParallelism[0] + "-" + config.TerraformCommandsNeedInput[0]

	helpers.RunTerragrunt(
		t,
		"terragrunt stack generate --working-dir "+filepath.Join(workingDir, "live")+commandsVariableExperimentFlag,
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt find --dag --dependencies --json --non-interactive --working-dir "+workingDir+
			commandsVariableExperimentFlag,
	)
	require.NoError(t, err)

	var components []struct {
		Path         string   `json:"path"`
		Dependencies []string `json:"dependencies"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	dependencies := map[string][]string{}

	for _, component := range components {
		for _, dependency := range component.Dependencies {
			path := filepath.ToSlash(component.Path)
			dependencies[path] = append(dependencies[path], filepath.ToSlash(dependency))
		}
	}

	assert.Equal(
		t,
		map[string][]string{"consumer": {"live/" + config.StackDir + "/" + unitName}},
		dependencies,
	)
}

func TestCommandsVariableRequiresExperiment(t *testing.T) {
	t.Parallel()

	helpers.SkipInExperimentMode(t, experiment.CommandsVariable)

	testCases := []struct {
		name string
		unit string
	}{
		{name: "locals", unit: filepath.Join("disabled", "locals")},
		{name: "inputs", unit: filepath.Join("disabled", "inputs")},
		{name: "values file", unit: filepath.Join("disabled", "values")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := copyCommandsVariableFixture(t)

			_, _, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt render --format json --non-interactive --working-dir "+filepath.Join(root, tc.unit),
			)

			var diags hcl.Diagnostics

			require.ErrorAs(t, err, &diags)
			assert.True(t, slices.ContainsFunc(diags, func(diag *hcl.Diagnostic) bool {
				return strings.Contains(diag.Detail, "--experiment "+experiment.CommandsVariable)
			}))
		})
	}
}

func copyCommandsVariableFixture(t *testing.T) string {
	t.Helper()

	tmpEnvPath, err := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureCommandsVariable))
	require.NoError(t, err)

	return filepath.Join(tmpEnvPath, testFixtureCommandsVariable)
}

func renderCommandsVariableInputs(t *testing.T, unitDir, flags string) map[string]any {
	t.Helper()

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --format json --non-interactive --working-dir "+unitDir+flags,
	)
	require.NoError(t, err)

	return commandsVariableInputs(t, []byte(stdout))
}

func commandsVariableInputs(t *testing.T, renderedJSON []byte) map[string]any {
	t.Helper()

	var rendered struct {
		Inputs map[string]any `json:"inputs"`
	}

	require.NoError(t, json.Unmarshal(renderedJSON, &rendered))

	return rendered.Inputs
}

func commandList(items []string) []any {
	list := make([]any, 0, len(items))
	for _, item := range items {
		list = append(list, item)
	}

	return list
}
