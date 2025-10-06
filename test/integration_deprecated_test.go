package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt hclvalidate --working-dir %s --json --show-config-path", rootPath))
	require.Error(t, err)

	var actualPaths []string

	err = json.Unmarshal([]byte(strings.TrimSpace(stdout)), &actualPaths)
	require.NoError(t, err)

	assert.ElementsMatch(t, expectedPaths, actualPaths)
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
		cmd := fmt.Sprintf("terragrunt %s --non-interactive --log-level trace --working-dir %s", strings.Join(tc.command, " "), testFixtureExtraArgsPath)

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

func TestDeprecatedRenderJsonCommand_RenderJsonDependentModulesMetadataTerraform(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDestroyWarning)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	tmpDir := util.JoinPath(tmpEnvPath, testFixtureDestroyWarning, "vpc")

	jsonOut := filepath.Join(tmpDir, "terragrunt_rendered.json")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt render-json --with-metadata --non-interactive --log-level trace --working-dir %s  --json-out %s", tmpDir, jsonOut))

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
