package test_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureStartswith              = "fixture-startswith"
	testFixtureTimecmp                 = "fixture-timecmp"
	testFixtureTimecmpInvalidTimestamp = "fixture-timecmp-errors/invalid-timestamp"
	testFixtureEndswith                = "fixture-endswith"
	testFixtureStrcontains             = "fixture-strcontains"
)

func TestStartsWith(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureStartswith)
	tmpEnvPath := copyEnvironment(t, testFixtureStartswith)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStartswith)

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	validateOutput(t, outputs, "startswith1", true)
	validateOutput(t, outputs, "startswith2", false)
	validateOutput(t, outputs, "startswith3", true)
	validateOutput(t, outputs, "startswith4", false)
	validateOutput(t, outputs, "startswith5", true)
	validateOutput(t, outputs, "startswith6", false)
	validateOutput(t, outputs, "startswith7", true)
	validateOutput(t, outputs, "startswith8", false)
	validateOutput(t, outputs, "startswith9", false)
}

func TestTimeCmp(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureTimecmp)
	tmpEnvPath := copyEnvironment(t, testFixtureTimecmp)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureTimecmp)

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	validateOutput(t, outputs, "timecmp1", float64(0))
	validateOutput(t, outputs, "timecmp2", float64(0))
	validateOutput(t, outputs, "timecmp3", float64(1))
	validateOutput(t, outputs, "timecmp4", float64(-1))
	validateOutput(t, outputs, "timecmp5", float64(-1))
	validateOutput(t, outputs, "timecmp6", float64(1))
}

func TestTimeCmpInvalidTimestamp(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureTimecmpInvalidTimestamp)
	tmpEnvPath := copyEnvironment(t, testFixtureTimecmpInvalidTimestamp)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureTimecmpInvalidTimestamp)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)

	expectedError := `not a valid RFC3339 timestamp: missing required time introducer 'T'`
	require.ErrorContains(t, err, expectedError)
}

func TestEndsWith(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureEndswith)
	tmpEnvPath := copyEnvironment(t, testFixtureEndswith)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureEndswith)

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	validateOutput(t, outputs, "endswith1", true)
	validateOutput(t, outputs, "endswith2", false)
	validateOutput(t, outputs, "endswith3", true)
	validateOutput(t, outputs, "endswith4", false)
	validateOutput(t, outputs, "endswith5", true)
	validateOutput(t, outputs, "endswith6", false)
	validateOutput(t, outputs, "endswith7", true)
	validateOutput(t, outputs, "endswith8", false)
	validateOutput(t, outputs, "endswith9", false)
}

func TestStrContains(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureStrcontains)
	tmpEnvPath := copyEnvironment(t, testFixtureStrcontains)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStrcontains)

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected outputs are not empty
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	validateOutput(t, outputs, "o1", true)
	validateOutput(t, outputs, "o2", false)
}
