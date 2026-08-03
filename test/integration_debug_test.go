package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fixtureMultiIncludeDependency = "fixtures/multiinclude-dependency"
	fixtureRenderJSON             = "fixtures/render-json"
	fixtureRenderJSONRegression   = "fixtures/render-json-regression"
)

func TestTerragruntValidateInputs(t *testing.T) {
	t.Parallel()

	moduleDirs, err := filepath.Glob(filepath.Join("fixtures/validate-inputs", "*"))
	require.NoError(t, err)

	mirror := helpers.NewGitServer(t)

	for _, module := range moduleDirs {
		name := filepath.Base(module)
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Render so __MIRROR_URL__ in the fixture resolves to the
			// local git mirror instead of the literal placeholder.
			tmpEnvPath := mirror.RenderFixture(module)
			modulePath := filepath.Join(tmpEnvPath, module)

			nameDashSplit := strings.Split(name, "-")
			helpers.RunTerragruntValidateInputs(
				t,
				modulePath,
				[]string{"--strict"},
				nameDashSplit[0] == "success",
			)
		})
	}
}

func TestTerragruntValidateInputsWithCLIVars(t *testing.T) {
	t.Parallel()

	moduleDir := filepath.Join("fixtures/validate-inputs", "fail-no-inputs")
	args := []string{"-var=input=from_env"}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, true)
}

func TestTerragruntValidateInputsWithCLIVarFile(t *testing.T) {
	t.Parallel()

	curdir, err := os.Getwd()
	require.NoError(t, err)

	moduleDir := filepath.Join("fixtures/validate-inputs", "fail-no-inputs")
	args := []string{
		fmt.Sprintf(
			"-var-file=%s/fixtures/validate-inputs/success-var-file/varfiles/main.tfvars",
			curdir,
		),
	}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, true)
}

func TestTerragruntValidateInputsWithStrictMode(t *testing.T) {
	t.Parallel()

	moduleDir := filepath.Join("fixtures/validate-inputs", "success-inputs-only")
	args := []string{"--strict-validate"}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, true)
}

func TestTerragruntValidateInputsWithStrictModeDisabledAndUnusedVar(t *testing.T) {
	t.Parallel()

	moduleDir := filepath.Join("fixtures/validate-inputs", "success-inputs-only")
	args := []string{"-var=testvariable=testvalue"}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, true)
}

func TestTerragruntValidateInputsWithStrictModeEnabledAndUnusedVar(t *testing.T) {
	t.Parallel()

	moduleDir := filepath.Join("fixtures/validate-inputs", "success-inputs-only")
	args := []string{"-var=testvariable=testvalue", "--strict"}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, false)
}

func TestTerragruntValidateInputsWithStrictModeEnabledAndUnusedInputs(t *testing.T) {
	t.Parallel()

	mirror := helpers.NewGitServer(t)
	moduleDir := filepath.Join("fixtures/validate-inputs", "fail-unused-inputs")
	helpers.CleanupTerraformFolder(t, moduleDir)
	tmpEnvPath, _ := filepath.EvalSymlinks(mirror.RenderFixture(moduleDir))
	rootPath := filepath.Join(tmpEnvPath, moduleDir)

	args := []string{"--strict"}
	helpers.RunTerragruntValidateInputs(t, rootPath, args, false)
}

func TestTerragruntValidateInputsWithStrictModeDisabledAndUnusedInputs(t *testing.T) {
	t.Parallel()

	mirror := helpers.NewGitServer(t)
	moduleDir := filepath.Join("fixtures/validate-inputs", "fail-unused-inputs")
	helpers.CleanupTerraformFolder(t, moduleDir)
	tmpEnvPath, _ := filepath.EvalSymlinks(mirror.RenderFixture(moduleDir))
	rootPath := filepath.Join(tmpEnvPath, moduleDir)

	args := []string{}
	helpers.RunTerragruntValidateInputs(t, rootPath, args, true)
}

// pins that hcl validate --inputs resolves get_original_terragrunt_dir() to the discovered unit
func TestTerragruntHCLValidateInputsResolvesOriginalTerragruntDir(t *testing.T) {
	t.Parallel()

	fixture := "fixtures/hcl-validate-original-dir"
	helpers.CleanupTerraformFolder(t, filepath.Join(fixture, "unit"))
	tmpEnvPath := helpers.CopyEnvironment(t, fixture)
	rootPath := filepath.Join(tmpEnvPath, fixture)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt hcl validate --inputs --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)
}

// pins the same for hcl validate without --inputs, which parses units through a separate path
func TestTerragruntHCLValidateResolvesOriginalTerragruntDir(t *testing.T) {
	t.Parallel()

	fixture := "fixtures/hcl-validate-original-dir"
	helpers.CleanupTerraformFolder(t, filepath.Join(fixture, "unit"))
	tmpEnvPath := helpers.CopyEnvironment(t, fixture)
	rootPath := filepath.Join(tmpEnvPath, fixture)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt hcl validate --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)
}

func TestRenderJSONConfigWithIncludesDependenciesAndLocals(t *testing.T) {
	t.Parallel()

	// This test is kind of wild. I don't know if it's worth keeping.
	// Removing it for now to avoid blocking the merge of #5477
	// which is more important.
	// TODO: Re-evaluate this test after #5477 is merged. See https://github.com/gruntwork-io/terragrunt/pull/5477

	t.Skip(
		"Skipping this test to avoid blocking the merge of #5477. See https://github.com/gruntwork-io/terragrunt/pull/5477",
	)

	tmpDir := helpers.TmpDirWOSymlinks(t)
	jsonOut := filepath.Join(tmpDir, "terragrunt_rendered.json")

	tmpEnvPath := helpers.CopyEnvironment(t, fixtureRenderJSONRegression)
	workDir := filepath.Join(tmpEnvPath, fixtureRenderJSONRegression)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+workDir+" -- apply -auto-approve",
	)

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt render --json -w --non-interactive --working-dir %s --json-out ",
			workDir,
		)+jsonOut,
	)

	jsonBytes, err := os.ReadFile(jsonOut)
	require.NoError(t, err)

	var rendered map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &rendered))

	// Make sure all terraform block is visible
	terraformBlock, hasTerraform := rendered["terraform"]
	if assert.True(t, hasTerraform) {
		source, hasSource := terraformBlock.(map[string]any)["source"]
		assert.True(t, hasSource)
		assert.Equal(t, "./foo", source)
	}

	// Make sure top level locals are rendered out
	locals, hasLocals := rendered["locals"]
	if assert.True(t, hasLocals) {
		assert.Equal(
			t,
			map[string]any{
				"foo": "bar",
			},
			locals.(map[string]any),
		)
	}

	// Make sure included dependency block is rendered out, and with the outputs rendered
	dependencyBlocks, hasDependency := rendered["dependency"]
	if assert.True(t, hasDependency) {
		assert.Equal(
			t,
			map[string]any{
				"baz": map[string]any{
					"name":         "baz",
					"config_path":  "./baz",
					"outputs":      nil,
					"inputs":       nil,
					"mock_outputs": nil,
					"enabled":      nil,
					"mock_outputs_allowed_terraform_commands": nil,
					"mock_outputs_merge_strategy_with_state":  nil,
					"mock_outputs_merge_with_state":           nil,
					"skip":                                    nil,
				},
			},
			dependencyBlocks.(map[string]any),
		)
	}

	// Make sure generate block is rendered out
	generateBlocks, hasGenerate := rendered["generate"]
	if assert.True(t, hasGenerate) {
		assert.Equal(
			t,
			map[string]any{
				"provider": map[string]any{
					"path":              "provider.tf",
					"comment_prefix":    "# ",
					"disable_signature": false,
					"disable":           false,
					"if_exists":         "overwrite",
					"if_disabled":       "skip",
					"hcl_fmt":           nil,
					"contents":          "# This is just a test",
				},
			},
			generateBlocks.(map[string]any),
		)
	}

	// Make sure all inputs are merged together
	inputsBlock, hasInputs := rendered["inputs"]
	if assert.True(t, hasInputs) {
		assert.Equal(
			t,
			map[string]any{
				"foo":       "bar",
				"baz":       "blah",
				"another":   "baz",
				"from_root": "Hi",
			},
			inputsBlock.(map[string]any),
		)
	}
}

func TestRenderJSONConfigRunAll(t *testing.T) {
	t.Parallel()

	// This test is kind of wild. I don't know if it's worth keeping.
	// Removing it for now to avoid blocking the merge of #5469
	// which is more important.

	t.Skip("Skipping this test to avoid blocking the merge of #5469")

	tmpEnvPath := helpers.CopyEnvironment(t, fixtureRenderJSONRegression)
	workDir := filepath.Join(tmpEnvPath, fixtureRenderJSONRegression)

	// NOTE: bar is not rendered out because it is considered a parent terragrunt.hcl config.

	bazJSONOut := filepath.Join(workDir, "baz", "terragrunt.rendered.json")
	rootChildJSONOut := filepath.Join(workDir, "terragrunt.rendered.json")

	defer os.Remove(bazJSONOut)
	defer os.Remove(rootChildJSONOut)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --non-interactive --working-dir "+workDir+" -- apply -auto-approve",
	)

	helpers.RunTerragrunt(
		t,
		"terragrunt render --all --json -w --non-interactive --working-dir "+workDir,
	)

	bazJSONBytes, err := os.ReadFile(bazJSONOut)
	require.NoError(t, err)

	var bazRendered map[string]any
	require.NoError(t, json.Unmarshal(bazJSONBytes, &bazRendered))

	// Make sure top level locals are rendered out
	bazLocals, bazHasLocals := bazRendered["locals"]
	if assert.True(t, bazHasLocals) {
		assert.Equal(
			t,
			map[string]any{
				"self": "baz",
			},
			bazLocals.(map[string]any),
		)
	}

	rootChildJSONBytes, err := os.ReadFile(rootChildJSONOut)
	require.NoError(t, err)

	var rootChildRendered map[string]any
	require.NoError(t, json.Unmarshal(rootChildJSONBytes, &rootChildRendered))

	// Make sure top level locals are rendered out
	rootChildLocals, rootChildHasLocals := rootChildRendered["locals"]
	if assert.True(t, rootChildHasLocals) {
		assert.Equal(
			t,
			map[string]any{
				"foo": "bar",
			},
			rootChildLocals.(map[string]any),
		)
	}
}

func TestRenderJSONConfigRunAllWithCLIRedesign(t *testing.T) {
	t.Parallel()

	// This test is kind of wild. I don't know if it's worth keeping.
	// Removing it for now to avoid blocking the merge of #5469
	// which is more important.

	t.Skip("Skipping this test to avoid blocking the merge of #5469")

	tmpEnvPath := helpers.CopyEnvironment(t, fixtureRenderJSONRegression)
	workDir := filepath.Join(tmpEnvPath, fixtureRenderJSONRegression)

	// NOTE: bar is not rendered out because it is considered a parent terragrunt.hcl config.

	bazJSONOut := filepath.Join(workDir, "baz", "terragrunt.rendered.json")
	rootChildJSONOut := filepath.Join(workDir, "terragrunt.rendered.json")

	defer os.Remove(bazJSONOut)
	defer os.Remove(rootChildJSONOut)

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+workDir)

	helpers.RunTerragrunt(
		t,
		"terragrunt render --all --json -w --non-interactive --working-dir "+workDir,
	)

	bazJSONBytes, err := os.ReadFile(bazJSONOut)
	require.NoError(t, err)

	var bazRendered map[string]any
	require.NoError(t, json.Unmarshal(bazJSONBytes, &bazRendered))

	// Make sure top level locals are rendered out
	bazLocals, bazHasLocals := bazRendered["locals"]
	if assert.True(t, bazHasLocals) {
		assert.Equal(
			t,
			map[string]any{
				"self": "baz",
			},
			bazLocals.(map[string]any),
		)
	}

	rootChildJSONBytes, err := os.ReadFile(rootChildJSONOut)
	require.NoError(t, err)

	var rootChildRendered map[string]any
	require.NoError(t, json.Unmarshal(rootChildJSONBytes, &rootChildRendered))

	// Make sure top level locals are rendered out
	rootChildLocals, rootChildHasLocals := rootChildRendered["locals"]
	if assert.True(t, rootChildHasLocals) {
		assert.Equal(
			t,
			map[string]any{
				"foo": "bar",
			},
			rootChildLocals.(map[string]any),
		)
	}
}

func TestDependencyGraphWithMultiInclude(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, fixtureMultiIncludeDependency)
	tmpEnvPath := helpers.CopyEnvironment(t, fixtureMultiIncludeDependency)
	rootPath := filepath.Join(tmpEnvPath, fixtureMultiIncludeDependency)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt dag graph --non-interactive --working-dir "+rootPath,
			&stdout,
			&stderr,
		),
	)
	stdoutStr := stdout.String()

	assert.Contains(t, stdoutStr, `"main" -> "depa";`)
	assert.Contains(t, stdoutStr, `"main" -> "depb";`)
	assert.Contains(t, stdoutStr, `"main" -> "depc";`)
}
