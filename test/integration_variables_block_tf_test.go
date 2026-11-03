//go:build tf

package test_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

func TestTFVariablesBlockSetsVariables(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "unit")

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --experiment variables-block --working-dir "+unitPath,
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -json values --non-interactive --experiment variables-block --working-dir "+unitPath,
	)
	require.NoError(t, err)

	var values map[string]string

	require.NoError(t, json.Unmarshal([]byte(stdout), &values))

	assert.Equal(t, map[string]string{
		"from_attr": "attr",
		"from_root": "root",
		"from_env":  "env",
		"shared":    "child",
		"template":  "${literal}",
		"nulled":    "null",
		"dropped":   "default",
	}, values)

	cacheDirs, err := filepath.Glob(filepath.Join(unitPath, ".terragrunt-cache", "*", "*", "main.tf"))
	require.NoError(t, err)
	require.Len(t, cacheDirs, 1)

	workingDir := filepath.Dir(cacheDirs[0])
	assert.FileExists(t, filepath.Join(workingDir, run.TFVarsFile))
	assert.FileExists(t, filepath.Join(workingDir, run.JSONVarsFile))
}

func TestTFVariablesBlockFilesOverrideTFVarEnv(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "unit")

	v := helpers.RunVenv(t)
	env := maps.Clone(v.Env)
	env["TF_VAR_from_root"] = "ambient"
	env["TF_VAR_shared"] = "ambient"
	env["TF_VAR_from_env"] = "ambient"
	v = v.WithEnv(env)

	_, _, err := helpers.RunTerragruntCommandWithOutputWithVenv(
		t,
		v,
		"terragrunt apply -auto-approve --non-interactive --experiment variables-block --working-dir "+unitPath,
	)
	require.NoError(t, err)

	stdout, _, err := helpers.RunTerragruntCommandWithOutputWithVenv(
		t,
		v,
		"terragrunt output -json values --non-interactive --experiment variables-block --working-dir "+unitPath,
	)
	require.NoError(t, err)

	var values map[string]string

	require.NoError(t, json.Unmarshal([]byte(stdout), &values))

	assert.Equal(t, "root", values["from_root"], "a tfvar input must override TF_VAR_from_root")
	assert.Equal(t, "child", values["shared"], "a json input must override TF_VAR_shared")
	assert.Equal(t, "ambient", values["from_env"], "TF_VAR_from_env must override an env input")
}

func TestTFVariablesBlockValidateRejectsUndeclaredVariables(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "undeclared")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --experiment variables-block --working-dir "+unitPath,
	)

	var undeclaredErr run.UndeclaredInputsError

	require.ErrorAs(t, err, &undeclaredErr)
	assert.Equal(t, []string{"undeclared"}, undeclaredErr.Names)
}

func TestTFVariablesBlockKindsPassTheSameValues(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"env", "tfvar", "json"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
			unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "values")

			v := helpers.RunVenv(t)
			env := maps.Clone(v.Env)
			env["TG_TEST_VARIABLES_KIND"] = kind
			v = v.WithEnv(env)

			_, _, err := helpers.RunTerragruntCommandWithOutputWithVenv(
				t,
				v,
				"terragrunt apply -auto-approve --non-interactive --experiment variables-block --working-dir "+unitPath,
			)
			require.NoError(t, err)

			stdout, _, err := helpers.RunTerragruntCommandWithOutputWithVenv(
				t,
				v,
				"terragrunt output -json values --non-interactive --experiment variables-block --working-dir "+unitPath,
			)
			require.NoError(t, err)

			assert.JSONEq(t, `{
				"number": 1.5,
				"large_number": 123456789012,
				"flag": false,
				"object": {"name": "app", "sizes": [1, 2]},
				"keyword_keys": {"for": "team", "if": "x"},
				"empty_list": [],
				"empty_map": {},
				"template": "${var.literal}",
				"directive": "%{if}",
				"multiple_lines": "line1\nline2\n",
				"unicode": "h\u00e9llo \"quoted\""
			}`, stdout)
		})
	}
}

func TestTFVariablesBlockNullable(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "nullable")

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --experiment variables-block --working-dir "+unitPath,
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -json values --non-interactive --experiment variables-block --working-dir "+unitPath,
	)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"env_kept": "null",
		"tfvar_kept": "null",
		"env_dropped": "default",
		"tfvar_dropped": "default"
	}`, stdout)
}

func TestTFVariablesBlockDependencyOutputs(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	fixturePath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "dependency")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --experiment variables-block --working-dir "+fixturePath+" -- plan",
	)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --experiment variables-block --working-dir "+fixturePath+
			" -- apply -auto-approve",
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -json --non-interactive --experiment variables-block --working-dir "+
			filepath.Join(fixturePath, "app"),
	)
	require.NoError(t, err)

	var outputs map[string]struct {
		Value any `json:"value"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "vpc-1", outputs["network_id"].Value)
	assert.Equal(t, map[string]any{"id": "vpc-1", "cidr": "10.0.0.0/16"}, outputs["network"].Value)
}

func TestTFVariablesBlockSavedPlan(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "unit")

	flags := " --non-interactive --experiment variables-block --working-dir " + unitPath

	helpers.RunTerragrunt(t, "terragrunt run"+flags+" -- plan -out=tf.plan")
	helpers.RunTerragrunt(t, "terragrunt run"+flags+" -- apply tf.plan")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -json values"+flags)
	require.NoError(t, err)

	var values map[string]string

	require.NoError(t, json.Unmarshal([]byte(stdout), &values))
	assert.Equal(t, "root", values["from_root"])
}

func TestTFVariablesBlockRunsInPlace(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "in-place")

	flags := " --non-interactive --experiment variables-block --experiment no-dot-terragrunt-cache --working-dir " +
		unitPath

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve"+flags)

	assert.NoDirExists(t, filepath.Join(unitPath, ".terragrunt-cache"))
	assert.NoFileExists(t, filepath.Join(unitPath, run.TFVarsFile))
	assert.NoFileExists(t, filepath.Join(unitPath, run.JSONVarsFile))

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -json values"+flags)
	require.NoError(t, err)

	assert.JSONEq(t, `{"from_tfvar": "tfvar", "from_json": "json"}`, stdout)
}

func TestTFVariablesBlockRemovedInputStopsApplying(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "kind-switch")
	configPath := filepath.Join(unitPath, config.DefaultTerragruntConfigPath)

	flags := " --non-interactive --experiment variables-block --working-dir " + unitPath

	require.NoError(t, os.WriteFile(configPath, []byte(`
variables {
  kind   = "tfvar"
  inputs = { name = "from-file" }
}
`), 0o644))
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve"+flags)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -raw name"+flags)
	require.NoError(t, err)
	assert.Equal(t, "from-file", stdout)

	require.NoError(t, os.WriteFile(configPath, nil, 0o644))
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve"+flags)

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -raw name"+flags)
	require.NoError(t, err)
	assert.Equal(t, "default", stdout)
}

func TestTFVariablesBlockAllowsUndeclaredFileInputWithoutValidate(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "undeclared-allowed")

	helpers.RunTerragrunt(
		t,
		"terragrunt plan --non-interactive --experiment variables-block --working-dir "+unitPath,
	)
}

func TestTFVariablesBlockLeavesUserFileAloneWithoutExperiment(t *testing.T) {
	t.Parallel()

	helpers.SkipInExperimentMode(t, experiment.VariablesBlock)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "reserved-name")

	flags := " --non-interactive --working-dir " + unitPath

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve"+flags)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -raw name"+flags)
	require.NoError(t, err)
	assert.Equal(t, "from-user-file", stdout)
}

func TestTFVariablesBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	helpers.SkipInExperimentMode(t, experiment.VariablesBlock)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "unit")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --working-dir "+unitPath,
	)

	var experimentErr config.VariablesBlockRequiresExperimentError

	require.ErrorAs(t, err, &experimentErr)
}
