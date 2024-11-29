package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureGetTerragruntSourceHcl = "fixtures/get-terragrunt-source-hcl"
)

func TestTerragruntSourceMap(t *testing.T) {
	t.Parallel()

	fixtureSourceMapPath := filepath.Join("fixtures", "source-map")
	helpers.CleanupTerraformFolder(t, fixtureSourceMapPath)
	tmpEnvPath := helpers.CopyEnvironment(t, fixtureSourceMapPath)
	rootPath := filepath.Join(tmpEnvPath, fixtureSourceMapPath)
	sourceMapArgs := fmt.Sprintf(
		"--terragrunt-source-map %s --terragrunt-source-map %s",
		"git::ssh://git@github.com/gruntwork-io/i-dont-exist.git="+tmpEnvPath,
		"git::ssh://git@github.com/gruntwork-io/another-dont-exist.git="+tmpEnvPath,
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

			tgArgs := fmt.Sprintf("terragrunt %s -auto-approve --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir %s %s", action, tgPath, sourceMapArgs)
			helpers.RunTerragrunt(t, tgArgs)
		})
	}
}

func TestGetTerragruntSourceHCL(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetTerragruntSourceHcl)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetTerragruntSourceHcl)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetTerragruntSourceHcl)
	terraformSource := "" // get_terragrunt_source_cli_flag() only returns the source when it is passed in via the CLI

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "HCL: "+terraformSource, outputs["terragrunt_source"].Value)
}

func TestGetTerragruntSourceCLI(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetTerragruntSourceCli)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetTerragruntSourceCli)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetTerragruntSourceCli)
	terraformSource := "terraform_config_cli"

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", rootPath, terraformSource))

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", rootPath, terraformSource), &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "CLI: "+terraformSource, outputs["terragrunt_source"].Value)
}
