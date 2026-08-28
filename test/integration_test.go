// Package test_test contains integration tests for Terragrunt.
package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hard-code this to match the test fixture for now
const (
	testFixtureAuthProviderCmd                = "fixtures/auth-provider-cmd"
	testFixtureAutoInit                       = "fixtures/download/init-on-source-change"
	testFixtureBrokenDependency               = "fixtures/broken-dependency"
	testFixtureBufferModuleOutput             = "fixtures/buffer-module-output"
	testFixtureCodegenPath                    = "fixtures/codegen"
	testFixtureCommandsThatNeedInput          = "fixtures/commands-that-need-input"
	testFixtureConfigSingleJSONPath           = "fixtures/config-files/single-json-config"
	testFixtureConfigWithNonDefaultNames      = "fixtures/config-files/with-non-default-names"
	testFixtureDependenciesOptimisation       = "fixtures/dependency-optimisation"
	testFixtureDependencyOutput               = "fixtures/dependency-output"
	testFixtureDetailedExitCode               = "fixtures/detailed-exitcode"
	testFixtureDirsPath                       = "fixtures/dirs"
	testFixtureDisabledModule                 = "fixtures/disabled/"
	testFixtureDisabledPath                   = "fixtures/disabled-path/"
	testFixtureDisjoint                       = "fixtures/stack/disjoint"
	testFixtureDownload                       = "fixtures/download"
	testFixtureEmptyState                     = "fixtures/empty-state/"
	testFixtureEnvVarsBlockPath               = "fixtures/env-vars-block/"
	testFixtureErrorPrint                     = "fixtures/error-print"
	testFixtureExcludesFile                   = "fixtures/excludes-file"
	testFixtureExternalDependence             = "fixtures/external-dependencies"
	testFixtureExternalDependency             = "fixtures/external-dependency/"
	testFixtureExtraArgsPath                  = "fixtures/extra-args/"
	testFixtureFailedTerraform                = "fixtures/failure"
	testFixtureFindParent                     = "fixtures/find-parent"
	testFixtureFindParentWithDeprecatedRoot   = "fixtures/find-parent-with-deprecated-root"
	testFixtureGetOutput                      = "fixtures/get-output"
	testFixtureGetTerragruntSourceCli         = "fixtures/get-terragrunt-source-cli"
	testFixtureRunAllSource                   = "fixtures/get-output/run-all-source"
	testFixtureGraphDependencies              = "fixtures/graph-dependencies"
	testFixtureHclfmtDiff                     = "fixtures/hclfmt-diff"
	testFixtureHclfmtStdin                    = "fixtures/hclfmt-stdin"
	testFixtureHclvalidate                    = "fixtures/hclvalidate"
	testFixtureIamRolesMultipleModules        = "fixtures/read-config/iam_roles_multiple_modules"
	testFixtureIncludeParent                  = "fixtures/include-parent"
	testFixtureInfoError                      = "fixtures/terragrunt-info-error"
	testFixtureInitCache                      = "fixtures/init-cache"
	testFixtureInitError                      = "fixtures/init-error"
	testFixtureInitOnce                       = "fixtures/init-once"
	testFixtureInputs                         = "fixtures/inputs"
	testFixtureInputsInterpolation            = "fixtures/inputs-interpolation"
	testFixtureLogFormatter                   = "fixtures/log/formatter"
	testFixtureLogStdoutLevel                 = "fixtures/log/levels"
	testFixtureLogRelPaths                    = "fixtures/log/rel-paths"
	testFixtureMissingDependence              = "fixtures/missing-dependencies/main"
	testFixtureModulePathError                = "fixtures/module-path-in-error"
	testFixtureNoColor                        = "fixtures/no-color"
	testFixtureNoSubmodules                   = "fixtures/no-submodules/"
	testFixtureNullValue                      = "fixtures/null-values"
	testFixtureOutDir                         = "fixtures/out-dir"
	testFixtureOutputAll                      = "fixtures/output-all"
	testFixtureParallelRun                    = "fixtures/parallel-run"
	testFixtureParallelStateInit              = "fixtures/parallel-state-init"
	testFixtureParallelism                    = "fixtures/parallelism"
	testFixturePath                           = "fixtures/terragrunt/"
	testFixturePlanfileOrder                  = "fixtures/planfile-order-test"
	testFixtureProviderCacheDependency        = "fixtures/provider-cache/dependency"
	testFixtureProviderCacheDirect            = "fixtures/provider-cache/direct"
	testFixtureProviderCacheFilesystemMirror  = "fixtures/provider-cache/filesystem-mirror"
	testFixtureProviderCacheFullLockfile      = "fixtures/provider-cache/full-lockfile"
	testFixtureProviderCacheMultiplePlatforms = "fixtures/provider-cache/multiple-platforms"
	testFixtureProviderCacheNetworkMirror     = "fixtures/provider-cache/network-mirror"
	testFixtureReadConfig                     = "fixtures/read-config"
	testFixtureRefSource                      = "fixtures/download/remote-ref"
	testFixtureSkip                           = "fixtures/skip/"
	testFixtureSkipLegacyRoot                 = "fixtures/skip-legacy-root/"
	testFixtureSkipDependencies               = "fixtures/skip-dependencies"
	testFixtureSourceMapSlashes               = "fixtures/source-map/slashes-in-ref"
	testFixtureStack                          = "fixtures/stack/"
	testFixtureStdout                         = "fixtures/download/stdout-test"
	testFixtureTfTest                         = "fixtures/tftest/"
	testFixtureExecCmd                        = "fixtures/exec-cmd"
	testFixtureExecCmdTfPath                  = "fixtures/exec-cmd-tf-path"
	testFixtureLogStreaming                   = "fixtures/streaming"
	testFixtureCLIFlagHints                   = "fixtures/cli-flag-hints"
	testFixtureEphemeralInputs                = "fixtures/ephemeral-inputs"
	testFixtureTfPathBasic                    = "fixtures/tf-path/basic"
	testFixtureTfPathTofuTerraform            = "fixtures/tf-path/tofu-terraform"
	testFixtureTraceParent                    = "fixtures/trace-parent"
	testFixtureVersionInvocation              = "fixtures/version-invocation"
	testFixtureVersionFilesCacheKey           = "fixtures/version-files-cache-key"
	testFixtureNoColorDependency              = "fixtures/no-color-dependency"
	hiddenRunAllFixturePath                   = "fixtures/hidden-runall"
)

func TestCLIFlagHints(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedError error
		args          string
	}{
		{
			expectedError: flags.NewGlobalFlagHintError("raw", "stack output", "raw"),
			args:          "-raw init",
		},
		{
			expectedError: flags.NewCommandFlagHintError(
				"run",
				"no-include-root",
				"catalog",
				"no-include-root",
			),
			args: "run --no-include-root",
		},
		{
			expectedError: flags.NewPassthroughFlagHintError("platform"),
			args:          "run --platform",
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureCLIFlagHints)
			rootPath := helpers.CopyEnvironment(t, testFixtureCLIFlagHints)
			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			_, _, err = helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt "+tc.args+" --working-dir "+rootPath,
			)
			assert.EqualError(t, err, tc.expectedError.Error())
		})
	}
}

func TestHclvalidateValidConfig(t *testing.T) {
	t.Parallel()

	t.Run("using --all", func(t *testing.T) {
		t.Parallel()
		helpers.CleanupTerraformFolder(t, testFixtureHclvalidate)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclvalidate)
		rootPath := filepath.Join(tmpEnvPath, testFixtureHclvalidate)

		_, _, err := helpers.RunTerragruntCommandWithOutput(
			t,
			"terragrunt hcl validate --all --strict --inputs --working-dir "+filepath.Join(
				rootPath,
				"valid",
			),
		)
		require.NoError(t, err)
	})

	t.Run("validate each individually", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureHclvalidate)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclvalidate)
		rootPath := filepath.Join(tmpEnvPath, testFixtureHclvalidate, "valid")

		// Test each subdirectory individually
		entries, err := os.ReadDir(rootPath)
		require.NoError(t, err)

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			subPath := filepath.Join(rootPath, entry.Name())

			t.Run(entry.Name(), func(t *testing.T) {
				t.Parallel()

				_, _, err := helpers.RunTerragruntCommandWithOutput(
					t,
					"terragrunt hcl validate --strict --inputs --working-dir "+subPath,
				)
				require.NoError(t, err)
			})
		}
	})
}

func TestHclvalidateDiagnostic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHclvalidate)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclvalidate)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHclvalidate)

	expectedDiags := diagnostic.Diagnostics{
		&diagnostic.Diagnostic{
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Invalid expression",
			Detail:   "Expected the start of an expression, but found an invalid expression token.",
			Range: &diagnostic.Range{
				Filename: filepath.Join(rootPath, "second/a/terragrunt.hcl"),
				Start:    diagnostic.Pos{Line: 2, Column: 6, Byte: 14},
				End:      diagnostic.Pos{Line: 3, Column: 1, Byte: 15},
			},
			Snippet: &diagnostic.Snippet{
				Context:              "locals",
				Code:                 "  t =\n}",
				StartLine:            2,
				HighlightStartOffset: 5,
				HighlightEndOffset:   6,
			},
		},
		&diagnostic.Diagnostic{
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Missing required argument",
			Detail:   "The argument \"config_path\" is required, but no definition was found.",
			Range: &diagnostic.Range{
				Filename: filepath.Join(rootPath, "second/c/terragrunt.hcl"),
				Start:    diagnostic.Pos{Line: 16, Column: 16, Byte: 219},
				End:      diagnostic.Pos{Line: 16, Column: 17, Byte: 220},
			},
			Snippet: &diagnostic.Snippet{
				Context:              "dependency \"iam\"",
				Code:                 "dependency iam {",
				StartLine:            16,
				HighlightStartOffset: 15,
				HighlightEndOffset:   16,
			},
		},
		&diagnostic.Diagnostic{
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Can't evaluate expression",
			Detail:   "You can only reference to other local variables here, but it looks like you're referencing something else (\"dependency\" is not defined)",
			Range: &diagnostic.Range{
				Filename: filepath.Join(rootPath, "second/c/terragrunt.hcl"),
				Start:    diagnostic.Pos{Line: 12, Column: 9, Byte: 149},
				End:      diagnostic.Pos{Line: 12, Column: 21, Byte: 161},
			},
			Snippet: &diagnostic.Snippet{
				Context:              "locals",
				Code:                 "  ddd = dependency.d",
				StartLine:            12,
				HighlightStartOffset: 8,
				HighlightEndOffset:   20,
			},
		},
		&diagnostic.Diagnostic{
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Can't evaluate expression",
			Detail:   "You can only reference to other local variables here, but it looks like you're referencing something else (\"dependency\" is not defined)",
			Range: &diagnostic.Range{
				Filename: filepath.Join(rootPath, "second/c/terragrunt.hcl"),
				Start:    diagnostic.Pos{Line: 10, Column: 9, Byte: 117},
				End:      diagnostic.Pos{Line: 10, Column: 31, Byte: 139},
			},
			Snippet: &diagnostic.Snippet{
				Context:              "locals",
				Code:                 "  vvv = dependency.a.outputs.z",
				StartLine:            10,
				HighlightStartOffset: 8,
				HighlightEndOffset:   30,
			},
		},
	}

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf("terragrunt hcl validate --working-dir %s --json", rootPath),
	)
	require.Error(t, err)

	var actualDiags diagnostic.Diagnostics

	err = json.Unmarshal([]byte(strings.TrimSpace(stdout)), &actualDiags)
	require.NoError(t, err)

	assert.ElementsMatch(t, expectedDiags, actualDiags)
}

func TestHclvalidateReturnsNonZeroExitCodeOnError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHclvalidate)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclvalidate)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHclvalidate)

	// We expect an error because the fixture has HCL validation issues.
	// The content of stdout and stderr isn't the primary focus here,
	// rather the fact that an error (non-zero exit code) is returned.
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt hcl validate --working-dir "+rootPath,
	)
	require.Error(
		t,
		err,
		"terragrunt hcl validate should return a non-zero exit code on HCL errors",
	)

	// As an additional check, we can verify that the error message indicates HCL validation errors.
	// This makes the test more robust.
	assert.Contains(t, err.Error(), "HCL validation error(s) found")
}

func TestHclvalidateInvalidConfigPath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHclvalidate)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclvalidate)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHclvalidate)

	expectedRelPaths := []string{
		filepath.Join("second", "a", "terragrunt.hcl"),
		filepath.Join("second", "c", "terragrunt.hcl"),
	}

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt hcl validate --working-dir %s --json --show-config-path",
			rootPath,
		),
	)
	require.Error(t, err)

	var actualPaths []string

	err = json.Unmarshal([]byte(strings.TrimSpace(stdout)), &actualPaths)
	require.NoError(t, err)

	for _, rel := range expectedRelPaths {
		found := false

		for _, p := range actualPaths {
			if strings.HasSuffix(p, rel) {
				found = true
				break
			}
		}

		assert.Truef(t, found, "expected a path ending with %q in %v", rel, actualPaths)
	}
}

func TestTerragruntGraphDependenciesCommand(t *testing.T) {
	t.Parallel()

	// this test doesn't even run plan, it exits right after the stack was created
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGraphDependencies)

	rootTerragruntConfigPath := filepath.Join(tmpEnvPath, testFixtureGraphDependencies, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(
		t,
		rootTerragruntConfigPath,
		rootTerragruntConfigPath,
		s3BucketName,
		"not-used",
		"not-used",
	)

	environmentPath := fmt.Sprintf("%s/%s/root", tmpEnvPath, testFixtureGraphDependencies)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	helpers.RunTerragruntRedirectOutput(
		t,
		"terragrunt dag graph --working-dir "+environmentPath,
		&stdout,
		&stderr,
	)
	output := stdout.String()
	assert.Contains(t, output, strings.TrimSpace(`
digraph {
	"backend-app" ;
	"backend-app" -> "mysql";
	"backend-app" -> "redis";
	"backend-app" -> "vpc";
	"frontend-app" ;
	"frontend-app" -> "backend-app";
	"frontend-app" -> "vpc";
	"mysql" ;
	"mysql" -> "vpc";
	"redis" ;
	"redis" -> "vpc";
	"vpc" ;
}
	`))
}

func validateInputs(t *testing.T, outputs map[string]helpers.TerraformOutput) {
	t.Helper()

	assert.Equal(t, true, outputs["bool"].Value)
	assert.Equal(t, []any{true, false}, outputs["list_bool"].Value)
	assert.Equal(t, []any{1.0, 2.0, 3.0}, outputs["list_number"].Value)
	assert.Equal(t, []any{"a", "b", "c"}, outputs["list_string"].Value)
	assert.Equal(
		t,
		map[string]any{"foo": true, "bar": false, "baz": true},
		outputs["map_bool"].Value,
	)
	assert.Equal(t, map[string]any{"foo": 42.0, "bar": 12345.0}, outputs["map_number"].Value)
	assert.Equal(t, map[string]any{"foo": "bar"}, outputs["map_string"].Value)
	assert.InEpsilon(t, 42.0, outputs["number"].Value, 0.0000000001)
	assert.Equal(
		t,
		map[string]any{
			"list": []any{1.0, 2.0, 3.0},
			"map":  map[string]any{"foo": "bar"},
			"num":  42.0,
			"str":  "string",
		},
		outputs["object"].Value,
	)
	assert.Equal(t, "string", outputs["string"].Value)
	assert.Equal(t, "default", outputs["from_env"].Value)
}

func TestDependencyInputsBlockedByDefault(t *testing.T) {
	t.Parallel()

	// Test that using dependency.foo.inputs syntax results in an error by default
	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Create a terragrunt.hcl that uses the deprecated dependency.foo.inputs syntax
	dependencyConfig := `
dependency "dep" {
  config_path = "../dep"
}

inputs = {
  # This should fail - dependency inputs are now blocked by default
  value = dependency.dep.inputs.some_value
}
`

	configPath := filepath.Join(tmpDir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(configPath, []byte(dependencyConfig), 0o644))

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	// Try to parse this config - it should fail with an error about dependency inputs
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt validate --non-interactive --working-dir "+tmpDir,
		&stdout,
		&stderr,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Reading inputs from dependencies is no longer supported")
	assert.Contains(t, err.Error(), "use outputs")
}

func TestShowErrorWhenRunAllInvokedWithoutArguments(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStack)
	appPath := filepath.Join(tmpEnvPath, testFixtureStack)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt run --all --non-interactive --working-dir "+appPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	var missingCommandError runall.MissingCommand

	ok := errors.As(err, &missingCommandError)
	assert.True(t, ok)
}

func TestHclFmtDiff(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHclfmtDiff)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclfmtDiff)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHclfmtDiff)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt hcl fmt --diff --working-dir "+rootPath,
			&stdout,
			&stderr,
		),
	)

	expectedDiff, err := os.ReadFile(filepath.Join(rootPath, "expected.diff"))
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, stdout, "output")

	// Drop the header lines that reference the temp-dir-qualified file path so
	// the hunk body can be compared exactly against the fixture.
	var hunk strings.Builder

	for line := range strings.SplitSeq(strings.TrimRight(stdout.String(), "\n"), "\n") {
		if strings.HasPrefix(line, "diff old/") || strings.HasPrefix(line, "--- old/") ||
			strings.HasPrefix(line, "+++ new/") {
			continue
		}

		hunk.WriteString(line)
		hunk.WriteByte('\n')
	}

	assert.Equal(t, strings.TrimRight(string(expectedDiff), "\n")+"\n", hunk.String())
}

func TestHclFmtStdin(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHclfmtStdin)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclfmtStdin)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHclfmtStdin)

	os.Stdin, _ = os.Open(filepath.Join(rootPath, "terragrunt.hcl"))

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt hcl fmt --stdin")
	require.NoError(t, err)

	expectedDiff, err := os.ReadFile(filepath.Join(rootPath, "expected.hcl"))
	require.NoError(t, err)

	assert.Contains(t, stdout, string(expectedDiff))
}

func TestTerragruntFailIfBucketCreationIsrequired(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath)
	helpers.CleanupTerraformFolder(t, rootPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(
		t,
		rootPath,
		s3BucketName,
		lockTableName,
		config.DefaultTerragruntConfigPath,
	)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		fmt.Sprintf(
			"terragrunt apply --fail-on-state-bucket-creation --non-interactive --config %s --working-dir %s",
			tmpTerragruntConfigPath,
			rootPath,
		),
		&stdout,
		&stderr,
	)
	require.Error(t, err)
}

func TestTerragruntInfoError(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInfoError)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureInfoError, "module-b")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt info print --non-interactive --working-dir "+testPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)

	// parse stdout json as InfoOutput
	var output print.InfoOutput

	err = json.Unmarshal(stdout.Bytes(), &output)
	require.NoError(t, err)
}

func TestUsingAllAndGraphFlagsSimultaneously(t *testing.T) {
	t.Parallel()

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --graph --all")
	expectedErr := new(shared.AllGraphFlagsError)
	require.ErrorAs(t, err, &expectedErr)
}

func TestErrorMessageIncludeInOutput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureErrorPrint)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureErrorPrint)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply  --non-interactive --working-dir "+testPath+" --tf-path "+testPath+"/custom-tf-script.sh --log-level trace",
	)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "Custom error from script")
}

//nolint:paralleltest
func TestTfPath(t *testing.T) {
	// This test can't be parallelized because it explicitly unsets the TG_TF_PATH environment variable.
	// t.Parallel()

	// Test that the terragrunt run version command correctly identifies and uses
	// the terraform_binary path configuration if present
	helpers.CleanupTerraformFolder(t, testFixtureTfPathBasic)
	rootPath := helpers.CopyEnvironment(t, testFixtureTfPathBasic)
	workingDir := filepath.Join(rootPath, testFixtureTfPathBasic)
	workingDir, err := filepath.EvalSymlinks(workingDir)
	require.NoError(t, err)

	// If TG_TF_PATH is not set, we'll use the default tofu binary,
	// we'll explicitly set the value so that the test can pass.
	if tfPath := os.Getenv("TG_TF_PATH"); tfPath != "" {
		// Unset after using t.Setenv so that it'll be reset after the test.
		t.Setenv("TG_TF_PATH", "")
		os.Unsetenv("TG_TF_PATH")
	}

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run version --working-dir "+workingDir,
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, "TF script used!")
}

func TestTfPathOverridesConfig(t *testing.T) {
	t.Parallel()
	// Test that the terragrunt run version command correctly identifies and uses
	// the terraform_binary path configuration if present
	helpers.CleanupTerraformFolder(t, testFixtureTfPathBasic)
	rootPath := helpers.CopyEnvironment(t, testFixtureTfPathBasic)
	workingDir := filepath.Join(rootPath, testFixtureTfPathBasic)
	workingDir, err := filepath.EvalSymlinks(workingDir)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run version --tf-path ./other-tf.sh --working-dir "+workingDir,
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Other TF script used!")
}

func TestTfPathOverridesConfigWithTofuTerraform(t *testing.T) {
	t.Parallel()

	// This test requires that both tofu and terraform are installed.
	if !helpers.IsTerraformInstalled(t.Context()) || !helpers.IsOpenTofuInstalled(t.Context()) {
		t.Skip("This test requires that both OpenTofu and Terraform are installed")

		return
	}

	helpers.CleanupTerraformFolder(t, testFixtureTfPathTofuTerraform)
	rootPath := helpers.CopyEnvironment(t, testFixtureTfPathTofuTerraform)
	workingDir := filepath.Join(rootPath, testFixtureTfPathTofuTerraform)
	workingDir, err := filepath.EvalSymlinks(workingDir)
	require.NoError(t, err)

	testCases := []struct {
		feature  string
		tfPath   string
		expected string
	}{
		{
			feature:  "tofu",
			tfPath:   helpers.TofuBinary,
			expected: "OpenTofu",
		},
		{
			feature:  "terraform",
			tfPath:   helpers.TerraformBinary,
			expected: "Terraform",
		},
		{
			feature:  "tofu",
			tfPath:   helpers.TerraformBinary,
			expected: "Terraform",
		},
		{
			feature:  "terraform",
			tfPath:   helpers.TofuBinary,
			expected: "OpenTofu",
		},
	}

	for _, tc := range testCases {
		stdout, _, err := helpers.RunTerragruntCommandWithOutput(
			t,
			fmt.Sprintf(
				"terragrunt run version --feature binary=%s --tf-path %s --working-dir %s",
				tc.feature,
				tc.tfPath,
				workingDir,
			),
		)
		require.NoError(t, err)

		assert.Contains(t, stdout, tc.expected)
	}
}

// Test that default command forwarding is disabled and users are guided to use `run --`.
func TestNoDefaultForwardingUnknownCommand(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt workspace list --non-interactive --working-dir "+rootPath,
	)
	require.Error(t, err, "expected error when invoking unknown top-level command without 'run'")
}

// TestTerragruntMutableGenerateBlock verifies that units generating identical
// contents share one read-only file, and that a block asking to stay mutable
// gets its own writable copy.
func TestTerragruntMutableGenerateBlock(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("read-only permission bits are not meaningfully observable on Windows")
	}

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	fixtureRoot := filepath.Join(tmpEnvPath, testFixtureCodegenPath, "mutable-generate")

	generated := map[string]os.FileInfo{}

	for _, unit := range []string{"unit-a", "unit-b", "unit-mutable"} {
		unitPath := filepath.Join(fixtureRoot, unit)
		helpers.CleanupTerraformFolder(t, unitPath)
		helpers.CleanupTerragruntFolder(t, unitPath)

		_, _, err := helpers.RunTerragruntCommandWithOutput(
			t,
			"terragrunt exec --experiment mutable-generate --working-dir "+unitPath+" -- true",
		)
		require.NoError(t, err)

		generated[unit] = statGeneratedFile(t, unitPath, "provider.tf")
	}

	assert.True(t, os.SameFile(generated["unit-a"], generated["unit-b"]),
		"units generating identical contents must share one file")
	assert.Equal(t, os.FileMode(0444), generated["unit-a"].Mode().Perm(),
		"deduplicated files must be read-only so an edit cannot reach the shared store")

	assert.False(t, os.SameFile(generated["unit-a"], generated["unit-mutable"]),
		"a mutable generate block must get its own file")
	assert.NotZero(t, generated["unit-mutable"].Mode().Perm()&0200,
		"a mutable generate block must stay writable")
}

// TestTerragruntMutableGenerateBlockRequiresExperiment verifies that the mutable
// attribute is rejected until the experiment gating it is enabled.
func TestTerragruntMutableGenerateBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip("Skipping: TG_EXPERIMENT_MODE forces all experiments on, so the experiment-disabled error this test pins cannot occur")
	}

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	unitPath := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"mutable-generate",
		"unit-mutable",
	)
	helpers.CleanupTerraformFolder(t, unitPath)
	helpers.CleanupTerragruntFolder(t, unitPath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt exec --working-dir "+unitPath+" -- true",
	)

	var experimentErr config.MutableGenerateRequiresExperimentError
	require.ErrorAs(t, err, &experimentErr)
}

// statGeneratedFile locates a generated file inside the unit's cache dir, which
// is nested under content-addressed directory names the test cannot predict.
func statGeneratedFile(t *testing.T, unitPath, name string) os.FileInfo {
	t.Helper()

	var found os.FileInfo

	require.NoError(t, filepath.WalkDir(
		filepath.Join(unitPath, util.TerragruntCacheDir),
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() || d.Name() != name {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return err
			}

			found = info

			return nil
		},
	))
	require.NotNil(t, found, "expected %s to be generated under %s", name, unitPath)

	return found
}
