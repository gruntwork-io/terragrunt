package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	print "github.com/gruntwork-io/terragrunt/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeprecatedHclvalidateCommand_HclvalidateInvalidConfigPath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHclvalidate)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclvalidate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclvalidate)

	expectedPaths := []string{
		filepath.Join(rootPath, "second/a/terragrunt.hcl"),
		filepath.Join(rootPath, "second/c/terragrunt.hcl"),
	}

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt hclvalidate --terragrunt-working-dir %s --terragrunt-hclvalidate-json --terragrunt-hclvalidate-show-config-path", rootPath))
	require.Error(t, err)

	var actualPaths []string

	err = json.Unmarshal([]byte(strings.TrimSpace(stdout)), &actualPaths)
	require.NoError(t, err)

	assert.ElementsMatch(t, expectedPaths, actualPaths)
}

func TestDeprecatedRunAllCommand_TerragruntReportsTerraformErrorsWithPlanAll(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFailedTerraform)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFailedTerraform)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, "fixtures/failure")

	cmd := "terragrunt run-all plan --terragrunt-non-interactive --terragrunt-working-dir " + rootTerragruntConfigPath
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	// Call helpers.RunTerragruntCommand directly because this command contains failures (which causes helpers.RunTerragruntRedirectOutput to abort) but we don't care.
	err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)
	require.NoError(t, err)

	output := stdout.String()
	errOutput := stderr.String()
	fmt.Printf("STDERR is %s.\n STDOUT is %s", errOutput, output)

	assert.Contains(t, errOutput, "missingvar1")
	assert.Contains(t, errOutput, "missingvar2")
}

func TestDeprecatedLegacyAllCommand_TerragruntStackCommandsWithPlanFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureDisjoint))
	require.NoError(t, err)
	disjointEnvironmentPath := util.JoinPath(tmpEnvPath, testFixtureDisjoint)

	helpers.CleanupTerraformFolder(t, disjointEnvironmentPath)
	helpers.RunTerragrunt(t, "terragrunt plan-all -out=plan.tfplan --terragrunt-log-level info --terragrunt-non-interactive --terragrunt-working-dir "+disjointEnvironmentPath)
	helpers.RunTerragrunt(t, "terragrunt run-all apply plan.tfplan --terragrunt-log-level info --terragrunt-non-interactive --terragrunt-working-dir "+disjointEnvironmentPath)
}

// This tests terragrunt properly passes through terraform commands with sub commands
// and any number of specified args
func TestDeprecatedDefaultCommand_TerraformSubcommandCliArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expected string
		command  []string
	}{
		{
			command:  []string{"force-unlock"},
			expected: wrappedBinary() + " force-unlock",
		},
		{
			command:  []string{"force-unlock", "foo"},
			expected: wrappedBinary() + " force-unlock foo",
		},
		{
			command:  []string{"force-unlock", "foo", "bar", "baz"},
			expected: wrappedBinary() + " force-unlock foo bar baz",
		},
		{
			command:  []string{"force-unlock", "foo", "bar", "baz", "foobar"},
			expected: wrappedBinary() + " force-unlock foo bar baz foobar",
		},
	}

	for _, tc := range testCases {
		cmd := fmt.Sprintf("terragrunt %s --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir %s", strings.Join(tc.command, " "), testFixtureExtraArgsPath)

		var (
			stdout bytes.Buffer
			stderr bytes.Buffer
		)
		// Call helpers.RunTerragruntCommand directly because this command contains failures (which causes helpers.RunTerragruntRedirectOutput to abort) but we don't care.
		if err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr); err == nil {
			t.Fatalf("Failed to properly fail command: %v.", cmd)
		}
		output := stdout.String()
		errOutput := stderr.String()
		assert.True(t, strings.Contains(errOutput, tc.expected) || strings.Contains(output, tc.expected))
	}
}

func TestDeprecatedTerragruntInfoCommand_TerragruntInfoError(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInfoError)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureInfoError, "module-b")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt terragrunt-info --terragrunt-non-interactive --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	// parse stdout json as InfoOutput
	var output print.InfoOutput
	err = json.Unmarshal(stdout.Bytes(), &output)
	require.NoError(t, err)
}

func TestDeprecatedRenderJsonCommand_RenderJsonDependentModulesMetadataTerraform(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDestroyWarning)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, testFixtureDestroyWarning, "vpc")

	jsonOut := filepath.Join(tmpDir, "terragrunt_rendered.json")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render-json --with-metadata --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir %s  --terragrunt-json-out %s", tmpDir, jsonOut))

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var renderedJSON = map[string]map[string]any{}

	require.NoError(t, json.Unmarshal(jsonBytes, &renderedJSON))

	dependentModules := renderedJSON[config.MetadataDependentModules]["value"].([]any)
	// check if value list contains app-v1 and app-v2
	assert.Contains(t, dependentModules, util.JoinPath(tmpEnvPath, testFixtureDestroyWarning, "app-v1"))
	assert.Contains(t, dependentModules, util.JoinPath(tmpEnvPath, testFixtureDestroyWarning, "app-v2"))
}

func TestDeprecatedHclFmtCommand_HclFmtDiff(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHclfmtDiff)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclfmtDiff)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclfmtDiff)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt hclfmt --diff --working-dir "+rootPath, &stdout, &stderr),
	)

	output := stdout.String()

	expectedDiff, err := os.ReadFile(util.JoinPath(rootPath, "expected.diff"))
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, stdout, "output")
	assert.Contains(t, output, string(expectedDiff))
}
