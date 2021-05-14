package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerragruntSourceMap(t *testing.T) {
	t.Parallel()

	fixtureSourceMapPath := "fixture-source-map"
	cleanupTerraformFolder(t, fixtureSourceMapPath)
	tmpEnvPath := copyEnvironment(t, fixtureSourceMapPath)
	rootPath := filepath.Join(tmpEnvPath, fixtureSourceMapPath)
	sourceMapArgs := fmt.Sprintf(
		"--terragrunt-source-map %s --terragrunt-source-map %s",
		fmt.Sprintf("git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=%s", tmpEnvPath),
		fmt.Sprintf("git::ssh://git@github.com/gruntwork-io/another-dont-exist.git=%s", tmpEnvPath),
	)

	testCases := []struct {
		name     string
		applyAll bool
	}{
		{
			name:     "multiple-match",
			applyAll: true,
		},
		{
			name:     "multiple-only-one-match",
			applyAll: true,
		},
		{
			name:     "multiple-with-dependency",
			applyAll: true,
		},
		{
			name:     "multiple-with-dependency-same-url",
			applyAll: true,
		},
		{
			name:     "single",
			applyAll: false,
		},
	}

	for _, testCase := range testCases {
		// capture range variable to avoid it changing across for loop runs during goroutine transitions.
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			tgPath := filepath.Join(rootPath, testCase.name)

			action := "apply"
			if testCase.applyAll {
				action = "run-all apply"
			}

			tgArgs := fmt.Sprintf("terragrunt %s -auto-approve --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s %s", action, tgPath, sourceMapArgs)
			runTerragrunt(t, tgArgs)
		})
	}
}

func TestGetTerragruntSourceHCL(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_HCL)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_HCL)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_HCL)
	terraformSource := "" // get_terragrunt_source_cli_flag() only returns the source when it is passed in via the CLI

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath))

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}

	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	assert.Equal(t, fmt.Sprintf("HCL: %s", terraformSource), outputs["terragrunt_source"].Value)
}

func TestGetTerragruntSourceCLI(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_CLI)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_CLI)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_GET_TERRAGRUNT_SOURCE_CLI)
	terraformSource := "terraform_config_cli"

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", rootPath, terraformSource))

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", rootPath, terraformSource), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}

	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	assert.Equal(t, fmt.Sprintf("CLI: %s", terraformSource), outputs["terragrunt_source"].Value)
}
