package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	TERRAGRUNT_DEBUG_FILE = "terragrunt-debug.tfvars.json"
)

func TestDebugGeneratedInputs(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, TEST_FIXTURE_INPUTS)
	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_INPUTS)
	rootPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_INPUTS)

	runTerragrunt(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-debug --terragrunt-working-dir %s", rootPath))

	debugFile := util.JoinPath(rootPath, TERRAGRUNT_DEBUG_FILE)
	assert.True(t, util.FileExists(debugFile))

	// If the debug file is generated correctly, we should be able to run terraform apply using the generated var file
	// without going through terragrunt.
	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test")
	require.NoError(t, err)
	mockOptions.WorkingDir = rootPath
	require.NoError(
		t,
		shell.RunTerraformCommand(mockOptions, "apply", "-auto-approve", "-var-file", debugFile),
	)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir %s", rootPath), &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout.String()), &outputs))
	validateInputs(t, outputs)

	// Also make sure the undefined variable is not included in the json file
	debugJsonContents, err := ioutil.ReadFile(debugFile)
	require.NoError(t, err)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(debugJsonContents, &data))
	_, isDefined := data["undefined_var"]
	assert.False(t, isDefined)
}

func TestTerragruntValidateInputs(t *testing.T) {
	t.Parallel()

	moduleDirs, err := filepath.Glob(filepath.Join("fixture-validate-inputs", "*"))
	require.NoError(t, err)

	for _, module := range moduleDirs {
		// capture range var within range scope so it doesn't change as the tests are spun to the background in the
		// t.Parallel call.
		module := module

		name := filepath.Base(module)
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			nameDashSplit := strings.Split(name, "-")
			runTerragruntValidateInputs(t, module, nil, nameDashSplit[0] == "success")
		})
	}
}

func TestTerragruntValidateInputsWithCLIVars(t *testing.T) {
	t.Parallel()

	moduleDir := filepath.Join("fixture-validate-inputs", "fail-no-inputs")
	args := []string{"-var=input=from_env"}
	runTerragruntValidateInputs(t, moduleDir, args, true)
}

func TestTerragruntValidateInputsWithCLIVarFile(t *testing.T) {
	t.Parallel()

	curdir, err := os.Getwd()
	require.NoError(t, err)

	moduleDir := filepath.Join("fixture-validate-inputs", "fail-no-inputs")
	args := []string{fmt.Sprintf("-var-file=%s/fixture-validate-inputs/success-var-file/varfiles/main.tfvars", curdir)}
	runTerragruntValidateInputs(t, moduleDir, args, true)
}

func runTerragruntValidateInputs(t *testing.T, moduleDir string, extraArgs []string, isSuccessTest bool) {
	maybeNested := filepath.Join(moduleDir, "module")
	if util.FileExists(maybeNested) {
		// Nested module test case with included file, so run terragrunt from the nested module.
		moduleDir = maybeNested
	}

	cmd := fmt.Sprintf("terragrunt validate-inputs %s --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", strings.Join(extraArgs, " "), moduleDir)
	t.Logf("Command: %s", cmd)
	_, _, err := runTerragruntCommandWithOutput(t, cmd)
	if isSuccessTest {
		require.NoError(t, err)
	} else {
		require.Error(t, err)
	}
}
