// Package test_test contains integration tests for Terragrunt.
package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/cli/commands/common"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
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
	testFixtureProviderCacheDirect            = "fixtures/provider-cache/direct"
	testFixtureProviderCacheFilesystemMirror  = "fixtures/provider-cache/filesystem-mirror"
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
	textFixtureDisjointSymlinks               = "fixtures/stack/disjoint-symlinks"
	testFixtureLogStreaming                   = "fixtures/streaming"
	testFixtureCLIFlagHints                   = "fixtures/cli-flag-hints"
	testFixtureEphemeralInputs                = "fixtures/ephemeral-inputs"
	testFixtureTfPathBasic                    = "fixtures/tf-path/basic"
	testFixtureTfPathTofuTerraform            = "fixtures/tf-path/tofu-terraform"
	testFixtureTraceParent                    = "fixtures/trace-parent"
	testFixtureVersionInvocation              = "fixtures/version-invocation"
	testFixtureVersionFilesCacheKey           = "fixtures/version-files-cache-key"
	hiddenRunAllFixturePath                   = "fixtures/hidden-runall"

	terraformFolder = ".terraform"

	terraformState = "terraform.tfstate"

	terraformStateBackup = "terraform.tfstate.backup"
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
			expectedError: flags.NewCommandFlagHintError("run", "no-include-root", "catalog", "no-include-root"),
			args:          "run --no-include-root",
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureCLIFlagHints)
			rootPath := helpers.CopyEnvironment(t, testFixtureCLIFlagHints)
			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt "+tc.args+" --working-dir "+rootPath)
			assert.EqualError(t, err, tc.expectedError.Error())
		})
	}
}

func TestDetailedExitCodeError(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "error")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)

	var exitCode tf.DetailedExitCode

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, &exitCode)

	_, stderr, err := helpers.RunTerragruntCommandWithOutputWithContext(t, ctx, "terragrunt run --all --log-level trace --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode")
	require.NoError(t, err)
	assert.Contains(t, stderr, "not-existing-file.txt: no such file or directory")
	assert.Equal(t, 1, exitCode.Get())
}

func TestDetailedExitCodeChangesPresentAll(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "changes")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)

	var exitCode tf.DetailedExitCode

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, &exitCode)

	_, _, err := helpers.RunTerragruntCommandWithOutputWithContext(t, ctx, "terragrunt run --all --log-level trace --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode")
	require.NoError(t, err)
	assert.Equal(t, 2, exitCode.Get())
}

func TestDetailedExitCodeChangesUnit(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "changes")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)
	ctx := t.Context()

	_, _, err := helpers.RunTerragruntCommandWithOutputWithContext(t, ctx, "terragrunt run --all --log-level trace --non-interactive --working-dir "+rootPath+" -- apply")
	require.NoError(t, err)

	// delete example.txt from rootPath/app1 to have changes in one unit
	err = os.Remove(filepath.Join(rootPath, "app1", "example.txt"))
	require.NoError(t, err)

	// check that the exit code is 2 when there are changes in one unit
	var exitCode tf.DetailedExitCode

	ctx = tf.ContextWithDetailedExitCode(ctx, &exitCode)

	_, _, err = helpers.RunTerragruntCommandWithOutputWithContext(t, ctx, "terragrunt run --all --log-level trace --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode")
	require.NoError(t, err)
	assert.Equal(t, 2, exitCode.Get())
}

func TestDetailedExitCodeFailOnFirstRun(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "fail-on-first-run")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)

	var exitCode tf.DetailedExitCode

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, &exitCode)

	_, _, err := helpers.RunTerragruntCommandWithOutputWithContext(t, ctx, "terragrunt run --all --log-level trace --non-interactive --working-dir "+util.JoinPath(tmpEnvPath, testFixturePath)+" -- plan -detailed-exitcode")
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode.Get())
}

func TestDetailedExitCodeChangesPresentOne(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "changes")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)

	var exitCode tf.DetailedExitCode

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, &exitCode)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all apply --log-level trace --non-interactive --working-dir "+filepath.Join(rootPath, "app1"))
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutputWithContext(t, ctx, "terragrunt run --all --log-level trace --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode")
	require.NoError(t, err)
	assert.Equal(t, 2, exitCode.Get())
}

func TestDetailedExitCodeNoChanges(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "changes")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)

	var exitCode tf.DetailedExitCode

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, &exitCode)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all apply --log-level trace --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutputWithContext(t, ctx, "terragrunt run --all --log-level trace --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode")
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode.Get())
}

func TestRunAllDetailedExitCode_RetryableAfterDrift(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "runall-retry-after-drift")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)

	// Pre-apply the drift unit so it has a file, then delete it to ensure drift exists
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all apply --log-level trace --non-interactive --working-dir "+filepath.Join(rootPath, "app_drift"))
	require.NoError(t, err)
	err = os.Remove(filepath.Join(rootPath, "app_drift", "example.txt"))
	require.NoError(t, err)

	var exitCode tf.DetailedExitCode

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, &exitCode)

	_, _, err = helpers.RunTerragruntCommandWithOutputWithContext(t, ctx, "terragrunt run --all --log-level trace --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode")
	require.NoError(t, err)
	assert.Equal(t, 2, exitCode.Get())
}

func TestLogCustomFormatOutput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr        error
		logCustomFormat    string
		expectedStdOutRegs []*regexp.Regexp
		expectedStdErrRegs []*regexp.Regexp
	}{
		{
			logCustomFormat: "%interval%(content=' plain-text ')%level(case=upper,width=6) %prefix(path=short-relative,suffix=' ')%tf-path(suffix=' ')%tf-command-args(suffix=': ')%msg(path=relative)",
			expectedStdOutRegs: []*regexp.Regexp{
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text STDOUT dep "+wrappedBinary()+" init -input=false -no-color: Initializing the backend...")),
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text STDOUT app "+wrappedBinary()+" init -input=false -no-color: Initializing the backend...")),
			},
			expectedStdErrRegs: []*regexp.Regexp{
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text DEBUG  Terragrunt Version:")),
			},
		},
		{
			logCustomFormat: "%interval%(content=' plain-text ')%level(case=upper,width=6) %prefix(path=short-relative,suffix=' ')%tf-path(suffix=' ')%tf-command(suffix=': ')%msg(path=relative)",
			expectedStdOutRegs: []*regexp.Regexp{
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text STDOUT dep "+wrappedBinary()+" init: Initializing the backend...")),
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text STDOUT app "+wrappedBinary()+" init: Initializing the backend...")),
			},
			expectedStdErrRegs: []*regexp.Regexp{
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text DEBUG  Terragrunt Version:")),
			},
		},
		{
			logCustomFormat: "%interval%(content=' plain-text ')%level(case=upper,width=6) %prefix(path=short-relative,suffix=' ')%tf-path(suffix=' ')%tf-command()-args %msg(path=relative)",
			expectedStdOutRegs: []*regexp.Regexp{
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text STDOUT dep "+wrappedBinary()+" init-args Initializing the backend...")),
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text STDOUT app "+wrappedBinary()+" init-args Initializing the backend...")),
			},
			expectedStdErrRegs: []*regexp.Regexp{
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text DEBUG  -args Terragrunt Version:")),
			},
		},
		{
			logCustomFormat: "%interval%(content=' plain-text ')%level(case=upper,width=6) %prefix(path=short-relative,suffix=' ')%tf-path(suffix=' ')%tf-command()-args % aaa %msg(path=relative) %%bbb % ccc",
			expectedStdOutRegs: []*regexp.Regexp{
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text STDOUT dep "+wrappedBinary()+" init-args % aaa Initializing the backend... %bbb % ccc")),
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text STDOUT app "+wrappedBinary()+" init-args % aaa Initializing the backend... %bbb % ccc")),
			},
			expectedStdErrRegs: []*regexp.Regexp{
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" plain-text DEBUG  -args % aaa Terragrunt Version:")),
			},
		},
		{
			logCustomFormat: "%time(color=green) %level %wrong",
			expectedErr:     errors.Errorf(`invalid value "%%time(color=green) %%level %%wrong" for flag -log-custom-format: invalid placeholder name "wrong", available names: %s`, strings.Join(placeholders.NewPlaceholderRegister().Names(), ",")),
		},
		{
			logCustomFormat: "%time(colorr=green) %level",
			expectedErr:     errors.Errorf(`invalid value "%%time(colorr=green) %%level" for flag -log-custom-format: placeholder "time", invalid option name "colorr", available names: %s`, strings.Join(placeholders.Time().Options().Names(), ",")),
		},
		{
			logCustomFormat: "%time(color=green) %level(format=tinyy)",
			expectedErr:     errors.New(`invalid value "%time(color=green) %level(format=tinyy)" for flag -log-custom-format: placeholder "level", option "format", invalid value "tinyy", available values: full,short,tiny`),
		},
		{
			logCustomFormat: "%time(=green) %level(format=tiny)",
			expectedErr:     errors.New(`invalid value "%time(=green) %level(format=tiny)" for flag -log-custom-format: placeholder "time", empty option name "=green) %level(format=tiny)"`),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all init --log-level trace --non-interactive -no-color --no-color --log-custom-format=%q --working-dir %s", tc.logCustomFormat, rootPath))

			if tc.expectedErr != nil {
				assert.EqualError(t, err, tc.expectedErr.Error())

				return
			}

			require.NoError(t, err)

			for _, reg := range tc.expectedStdOutRegs {
				assert.Regexp(t, reg, stdout)
			}

			for _, reg := range tc.expectedStdErrRegs {
				assert.Regexp(t, reg, stderr)
			}
		})
	}
}

func TestBufferModuleOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureBufferModuleOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureBufferModuleOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureBufferModuleOutput)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --log-disable --working-dir "+rootPath+" -- plan -out planfile")
	require.NoError(t, err)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-disable --working-dir "+rootPath+" -- show -json planfile")
	require.NoError(t, err)

	for stdout := range strings.SplitSeq(stdout, "\n") {
		if stdout == "" {
			continue
		}

		var objmap map[string]json.RawMessage

		err = json.Unmarshal([]byte(stdout), &objmap)
		require.NoError(t, err)
	}
}

func TestDisableLogging(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all init --log-level trace --log-disable --non-interactive -no-color --no-color --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Initializing provider plugins...")
	assert.Empty(t, stderr)
}

func TestLogWithAbsPath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all init --log-level trace --log-show-abs-paths --non-interactive -no-color --no-color --log-format=pretty --working-dir "+rootPath)
	require.NoError(t, err)

	for _, prefixName := range []string{"app", "dep"} {
		prefixName = filepath.Join(rootPath, prefixName)
		assert.Contains(t, stdout, "STDOUT ["+prefixName+"] "+wrappedBinary()+": Initializing provider plugins...")
		assert.Contains(t, stderr, "DEBUG  ["+prefixName+"] Reading Terragrunt config file at "+prefixName+"/terragrunt.hcl")
	}
}

func TestLogWithRelPath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogRelPaths)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogRelPaths)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogRelPaths)

	testCases := []struct {
		assertFn   func(t *testing.T, stdout, stderr string)
		workingDir string
	}{
		{
			workingDir: "duplicate-dir-names/workspace/one/two/aaa", // dir `workspace` duplicated twice in path
			assertFn: func(t *testing.T, _, stderr string) {
				t.Helper()

				assert.Contains(t, stderr, "Unit bbb/ccc/workspace")
				assert.Contains(t, stderr, "Unit bbb/ccc/module-b")
				assert.Contains(t, stderr, "Downloading Terraform configurations from .. into ./bbb/ccc/workspace/.terragrunt-cache")
				assert.Contains(t, stderr, "[bbb/ccc/workspace]")
				assert.Contains(t, stderr, "[bbb/ccc/module-b]")
			},
		},
	}

	for i, tc := range testCases {
		workingDir := filepath.Join(rootPath, tc.workingDir)

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all init --log-level trace --non-interactive --no-color --log-format=pretty --working-dir "+workingDir)
			require.NoError(t, err)

			tc.assertFn(t, stdout, stderr)
		})
	}
}

func TestLogFormatPrettyOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all init --log-level trace --non-interactive --no-color --log-format=pretty  --working-dir "+rootPath)
	require.NoError(t, err)

	for _, prefixName := range []string{"app", "dep"} {
		assert.Contains(t, stdout, "STDOUT ["+prefixName+"] "+wrappedBinary()+": Initializing provider plugins...")
		assert.Contains(t, stderr, "DEBUG  ["+prefixName+"] Reading Terragrunt config file at ./"+prefixName+"/terragrunt.hcl")
	}

	assert.NotEmpty(t, stdout)
	assert.Contains(t, stderr, "DEBUG  Terragrunt Version:")
}

func TestLogStdoutLevel(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogStdoutLevel)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogStdoutLevel)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogStdoutLevel)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive -no-color --no-color --log-format=pretty  --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "STDOUT "+wrappedBinary()+": Changes to Outputs")

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt destroy -auto-approve --non-interactive -no-color --no-color --log-format=pretty  --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "STDOUT "+wrappedBinary()+": Changes to Outputs")
}

func TestLogFormatKeyValueOutput(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{"--log-format=key-value"} {
		t.Run("tc-flag-"+flag, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --log-level trace --non-interactive "+flag+" --working-dir "+rootPath+" -- init -no-color")
			require.NoError(t, err)

			for _, prefixName := range []string{"app", "dep"} {
				assert.Contains(t, stdout, "level=stdout prefix="+prefixName+" tf-path="+wrappedBinary()+" msg=Initializing provider plugins...\n")
				assert.Contains(t, stderr, "level=debug prefix="+prefixName+" msg=Reading Terragrunt config file at ./"+prefixName+"/terragrunt.hcl\n")
			}
		})
	}
}

func TestLogRawModuleOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --log-level trace --non-interactive  --tf-forward-stdout --working-dir "+rootPath+" -- init -no-color")
	require.NoError(t, err)

	stdoutInline := strings.ReplaceAll(stdout, "\n", "")
	assert.Contains(t, stdoutInline, "Initializing the backend...Initializing provider plugins...")
	assert.NotRegexp(t, `(?i)(`+strings.Join(log.AllLevels.Names(), "|")+`)+`, stdoutInline)
}

func TestTerragruntExcludesFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		flags          string
		expectedOutput []string
	}{
		{
			"",
			[]string{`value = "b"`, `value = "d"`},
		},
		{
			"--queue-excludes-file ./excludes-file-pass-as-flag",
			[]string{`value = "a"`, `value = "c"`},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExcludesFile, ".terragrunt-excludes")
			rootPath := util.JoinPath(tmpEnvPath, testFixtureExcludesFile)

			helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run apply --all --non-interactive --working-dir %s %s -- -auto-approve", rootPath, tc.flags))

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run output --all --non-interactive --working-dir %s %s", rootPath, tc.flags))
			require.NoError(t, err)

			actualOutput := strings.Split(strings.TrimSpace(stdout), "\n")
			assert.ElementsMatch(t, tc.expectedOutput, actualOutput)
		})
	}
}

func TestHclvalidateValidConfig(t *testing.T) {
	t.Parallel()

	t.Run("using --all", func(t *testing.T) {
		t.Parallel()
		helpers.CleanupTerraformFolder(t, testFixtureHclvalidate)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclvalidate)
		rootPath := util.JoinPath(tmpEnvPath, testFixtureHclvalidate)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt hcl validate --all --strict --inputs --working-dir "+filepath.Join(rootPath, "valid"))
		require.NoError(t, err)
	})

	t.Run("validate each individually", func(t *testing.T) {
		t.Parallel()

		helpers.CleanupTerraformFolder(t, testFixtureHclvalidate)
		tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclvalidate)
		rootPath := util.JoinPath(tmpEnvPath, testFixtureHclvalidate, "valid")

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

				_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt hcl validate --strict --inputs --working-dir "+subPath)
				require.NoError(t, err)
			})
		}
	})
}

func TestHclvalidateDiagnostic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHclvalidate)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclvalidate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclvalidate)

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
			Summary:  "Unsupported attribute",
			Detail:   "This object does not have an attribute named \"outputs\".",
			Range: &diagnostic.Range{
				Filename: filepath.Join(rootPath, "second/c/terragrunt.hcl"),
				Start:    diagnostic.Pos{Line: 6, Column: 19, Byte: 86},
				End:      diagnostic.Pos{Line: 6, Column: 27, Byte: 94},
			},
			Snippet: &diagnostic.Snippet{
				Context:              "",
				Code:                 "  c = dependency.a.outputs.z",
				StartLine:            6,
				HighlightStartOffset: 18,
				HighlightEndOffset:   26,
				Values:               []diagnostic.ExpressionValue{{Traversal: "dependency.a", Statement: "is object with no attributes"}},
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

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt hcl validate --working-dir %s --json", rootPath))
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
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclvalidate)

	// We expect an error because the fixture has HCL validation issues.
	// The content of stdout and stderr isn't the primary focus here,
	// rather the fact that an error (non-zero exit code) is returned.
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt hcl validate --working-dir "+rootPath)
	require.Error(t, err, "terragrunt hcl validate should return a non-zero exit code on HCL errors")

	// As an additional check, we can verify that the error message indicates HCL validation errors.
	// This makes the test more robust.
	assert.Contains(t, err.Error(), "HCL validation error(s) found")
}

func TestHclvalidateInvalidConfigPath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHclvalidate)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclvalidate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclvalidate)

	expectedRelPaths := []string{
		filepath.Join("second", "a", "terragrunt.hcl"),
		filepath.Join("second", "c", "terragrunt.hcl"),
	}

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt hcl validate --working-dir %s --json --show-config-path", rootPath))
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

func TestTerragruntProviderCacheMultiplePlatforms(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureProviderCacheMultiplePlatforms)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureProviderCacheMultiplePlatforms)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureProviderCacheMultiplePlatforms)

	providerCacheDir := t.TempDir()

	var (
		platforms     = []string{"linux_amd64", "darwin_arm64"}
		platformsArgs = make([]string, 0, len(platforms))
	)

	for _, platform := range platforms {
		platformsArgs = append(platformsArgs, "-platform="+platform)
	}

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run --all --no-auto-init --provider-cache --provider-cache-dir %s --log-level trace --non-interactive --working-dir %s", providerCacheDir, rootPath)+" -- providers lock "+strings.Join(platformsArgs, " "))

	providers := []string{
		"hashicorp/aws/5.36.0",
		"hashicorp/azurerm/3.95.0",
	}

	registryName := "registry.opentofu.org"
	if isTerraform() {
		registryName = "registry.terraform.io"
	}

	for _, appName := range []string{"app1", "app2", "app3"} {
		appPath := filepath.Join(rootPath, appName)
		assert.True(t, util.FileExists(appPath))

		lockfilePath := filepath.Join(appPath, ".terraform.lock.hcl")
		lockfileContent, err := os.ReadFile(lockfilePath)
		require.NoError(t, err)

		lockfile, diags := hclwrite.ParseConfig(lockfileContent, lockfilePath, hcl.Pos{Line: 1, Column: 1})
		assert.False(t, diags.HasErrors())
		assert.NotNil(t, lockfile)

		for _, provider := range providers {
			provider := path.Join(registryName, provider)

			providerBlock := lockfile.Body().FirstMatchingBlock("provider", []string{filepath.Dir(provider)})
			assert.NotNil(t, providerBlock)

			providerPath := filepath.Join(providerCacheDir, provider)
			assert.True(t, util.FileExists(providerPath))

			for _, platform := range platforms {
				platformPath := filepath.Join(providerPath, platform)
				assert.True(t, util.FileExists(platformPath))
			}
		}
	}
}

func TestTerragruntInitOnce(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInitOnce)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureInitOnce)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+rootPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Initializing modules")

	// update the config creation time without changing content
	cfgPath := filepath.Join(rootPath, "terragrunt.hcl")
	bytes, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	err = os.WriteFile(cfgPath, bytes, 0644)
	require.NoError(t, err)

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+rootPath)
	require.NoError(t, err)
	assert.NotContains(t, stdout, "Initializing modules", "init command executed more than once")
}

func TestTerragruntWorksWithSingleJsonConfig(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureConfigSingleJSONPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureConfigSingleJSONPath)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureConfigSingleJSONPath)

	helpers.RunTerragrunt(t, "terragrunt plan --non-interactive --working-dir "+rootTerragruntConfigPath)
}

func TestTerragruntWorksWithNonDefaultConfigNamesAndRunAllCommand(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureConfigWithNonDefaultNames)
	tmpEnvPath = path.Join(tmpEnvPath, testFixtureConfigWithNonDefaultNames)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all apply --log-level debug --config main.hcl --non-interactive --working-dir "+tmpEnvPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "run_cmd output: [parent_hcl_file]")
	assert.Contains(t, stderr, "run_cmd output: [dependency_hcl]")
	assert.Contains(t, stderr, "run_cmd output: [common_hcl]")
}

func TestTerragruntWorksWithNonDefaultConfigNames(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureConfigWithNonDefaultNames)
	tmpEnvPath = path.Join(tmpEnvPath, testFixtureConfigWithNonDefaultNames)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply --config main.hcl --non-interactive --working-dir "+filepath.Join(tmpEnvPath, "app"), &stdout, &stderr)
	require.NoError(t, err)

	out := stdout.String()
	assert.Equal(t, 1, strings.Count(out, "parent_hcl_file"))
	assert.Equal(t, 1, strings.Count(out, "dependency_hcl"))
	assert.Equal(t, 1, strings.Count(out, "common_hcl"))
}

func TestTerragruntReportsTerraformErrorsWithPlanAll(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFailedTerraform)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFailedTerraform)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, "fixtures/failure")

	cmd := "terragrunt run --all plan --non-interactive --working-dir " + rootTerragruntConfigPath

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

func TestTerragruntGraphDependenciesCommand(t *testing.T) {
	t.Parallel()

	// this test doesn't even run plan, it exits right after the stack was created
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGraphDependencies)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureGraphDependencies, "root.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/root", tmpEnvPath, testFixtureGraphDependencies)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	helpers.RunTerragruntRedirectOutput(t, "terragrunt dag graph --working-dir "+environmentPath, &stdout, &stderr)
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

// Check that Terragrunt does not pollute stdout with anything
func TestTerragruntStdOut(t *testing.T) {
	t.Parallel()

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureStdout)
	helpers.RunTerragruntRedirectOutput(t, "terragrunt output foo --non-interactive --working-dir "+testFixtureStdout, &stdout, &stderr)

	output := stdout.String()
	assert.Equal(t, "\"foo\"\n", output)
}

func TestTerragruntStackCommandsWithPlanFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureDisjoint))
	require.NoError(t, err)

	disjointEnvironmentPath := util.JoinPath(tmpEnvPath, testFixtureDisjoint)

	helpers.CleanupTerraformFolder(t, disjointEnvironmentPath)
	helpers.RunTerragrunt(t, "terragrunt run --all  --log-level info --non-interactive --working-dir "+disjointEnvironmentPath+" -- plan -out=plan.tfplan")
	helpers.RunTerragrunt(t, "terragrunt run --all --log-level info --non-interactive --working-dir "+disjointEnvironmentPath+" -- apply plan.tfplan")
}

func TestTerragruntStackCommandsWithSymlinks(t *testing.T) {
	t.Parallel()

	// please be aware that helpers.CopyEnvironment resolves symlinks statically,
	// so the symlinked directories are copied physically, which defeats the purpose of this test,
	// therefore we are going to create the symlinks manually in the destination directory
	tmpEnvPath, err := filepath.EvalSymlinks(helpers.CopyEnvironment(t, textFixtureDisjointSymlinks))
	require.NoError(t, err)

	disjointSymlinksEnvironmentPath := util.JoinPath(tmpEnvPath, textFixtureDisjointSymlinks)
	require.NoError(t, os.Symlink(util.JoinPath(disjointSymlinksEnvironmentPath, "a"), util.JoinPath(disjointSymlinksEnvironmentPath, "b")))
	require.NoError(t, os.Symlink(util.JoinPath(disjointSymlinksEnvironmentPath, "a"), util.JoinPath(disjointSymlinksEnvironmentPath, "c")))

	helpers.CleanupTerraformFolder(t, disjointSymlinksEnvironmentPath)

	// perform the first initialization
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all init --experiment symlinks --log-level info --non-interactive --working-dir "+disjointSymlinksEnvironmentPath)
	require.NoError(t, err)
	assert.Contains(t, stderr, "Downloading Terraform configurations from ./module into ./a/.terragrunt-cache")
	assert.Contains(t, stderr, "Downloading Terraform configurations from ./module into ./b/.terragrunt-cache")
	assert.Contains(t, stderr, "Downloading Terraform configurations from ./module into ./c/.terragrunt-cache")

	// perform the second initialization and make sure that the cache is not downloaded again
	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all init --experiment symlinks --log-level info --non-interactive --working-dir "+disjointSymlinksEnvironmentPath)
	require.NoError(t, err)
	assert.NotContains(t, stderr, "Downloading Terraform configurations from ./module into ./a/.terragrunt-cache")
	assert.NotContains(t, stderr, "Downloading Terraform configurations from ./module into ./b/.terragrunt-cache")
	assert.NotContains(t, stderr, "Downloading Terraform configurations from ./module into ./c/.terragrunt-cache")

	// validate the modules
	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all validate --experiment symlinks --log-level info --non-interactive --working-dir "+disjointSymlinksEnvironmentPath)
	require.NoError(t, err)
	assert.Contains(t, stderr, "Unit a")
	assert.Contains(t, stderr, "Unit b")
	assert.Contains(t, stderr, "Unit c")

	// touch the "module/main.tf" file to change the timestamp and make sure that the cache is downloaded again
	require.NoError(t, os.Chtimes(util.JoinPath(disjointSymlinksEnvironmentPath, "module/main.tf"), time.Now(), time.Now()))

	// perform the initialization and make sure that the cache is downloaded again
	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all init --experiment symlinks --log-level info --non-interactive --working-dir "+disjointSymlinksEnvironmentPath)
	require.NoError(t, err)
	assert.Contains(t, stderr, "Downloading Terraform configurations from ./module into ./a/.terragrunt-cache")
	assert.Contains(t, stderr, "Downloading Terraform configurations from ./module into ./b/.terragrunt-cache")
	assert.Contains(t, stderr, "Downloading Terraform configurations from ./module into ./c/.terragrunt-cache")
}

func TestInvalidSource(t *testing.T) {
	t.Parallel()

	generateTestCase := testFixtureNotExistingSource
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt init --working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)

	var workingDirNotFoundErr run.WorkingDirNotFound

	ok := errors.As(err, &workingDirNotFoundErr)
	assert.True(t, ok)
}

func TestPlanfileOrder(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixturePlanfileOrder)
	modulePath := util.JoinPath(rootPath, testFixturePlanfileOrder)

	err := helpers.RunTerragruntCommand(t, "terragrunt plan --working-dir "+modulePath, os.Stdout, os.Stderr)
	require.NoError(t, err)

	err = helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --working-dir "+modulePath, os.Stdout, os.Stderr)
	require.NoError(t, err)
}

// This tests terragrunt properly passes through terraform commands and any number of specified args
func TestTerraformCommandCliArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr error
		expected    string
		command     []string
	}{
		{
			command:  []string{"version"},
			expected: wrappedBinary() + " version",
		},
		{
			command:  []string{"--", "version"},
			expected: wrappedBinary() + " version",
		},
		{
			command:  []string{"--", "version", "foo"},
			expected: wrappedBinary() + " version",
		},
		{
			command:  []string{"--", "version", "foo", "bar", "baz"},
			expected: wrappedBinary() + " version",
		},
		{
			command:  []string{"--", "version", "foo", "bar", "baz", "foobar"},
			expected: wrappedBinary() + " version",
		},
		{
			command:  []string{"--", "graph"},
			expected: "digraph",
		},
		{
			command:     []string{"--", "paln"}, //codespell:ignore
			expected:    "",
			expectedErr: expectedWrongCommandErr("paln"), //codespell:ignore
		},
		{
			command:  []string{"--disable-command-validation", "--", "paln"}, //codespell:ignore
			expected: "has no command named",                                 // error caused by running terraform with the wrong command
		},
	}

	for _, tc := range testCases {
		cmd := fmt.Sprintf("terragrunt run --non-interactive --log-level trace --working-dir %s %s", testFixtureExtraArgsPath, strings.Join(tc.command, " "))

		var (
			stdout bytes.Buffer
			stderr bytes.Buffer
		)

		err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)
		if tc.expectedErr != nil {
			require.ErrorIs(t, err, tc.expectedErr)
		}

		output := stdout.String()
		errOutput := stderr.String()
		assert.Contains(t, output+errOutput, tc.expected)
	}
}

// This tests terragrunt properly passes through terraform commands with sub commands
// and any number of specified args
func TestTerraformSubcommandCliArgs(t *testing.T) {
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

func validateInputs(t *testing.T, outputs map[string]helpers.TerraformOutput) {
	t.Helper()

	assert.Equal(t, true, outputs["bool"].Value)
	assert.Equal(t, []any{true, false}, outputs["list_bool"].Value)
	assert.Equal(t, []any{1.0, 2.0, 3.0}, outputs["list_number"].Value)
	assert.Equal(t, []any{"a", "b", "c"}, outputs["list_string"].Value)
	assert.Equal(t, map[string]any{"foo": true, "bar": false, "baz": true}, outputs["map_bool"].Value)
	assert.Equal(t, map[string]any{"foo": 42.0, "bar": 12345.0}, outputs["map_number"].Value)
	assert.Equal(t, map[string]any{"foo": "bar"}, outputs["map_string"].Value)
	assert.InEpsilon(t, 42.0, outputs["number"].Value, 0.0000000001)
	assert.Equal(t, map[string]any{"list": []any{1.0, 2.0, 3.0}, "map": map[string]any{"foo": "bar"}, "num": 42.0, "str": "string"}, outputs["object"].Value)
	assert.Equal(t, "string", outputs["string"].Value)
	assert.Equal(t, "default", outputs["from_env"].Value)
}

func TestInputsPassedThroughCorrectly(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureInputs)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	validateInputs(t, outputs)
}

func TestRunCommand(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureInputs)

	helpers.RunTerragrunt(t, "terragrunt run --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt run -no-color --non-interactive --working-dir "+rootPath+" -- output -json", &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	validateInputs(t, outputs)
}

func TestTerragruntMissingDependenciesFail(t *testing.T) {
	t.Parallel()

	generateTestCase := testFixtureMissingDependence
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt init --working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)

	var parsedError config.DependencyDirNotFoundError

	ok := errors.As(err, &parsedError)
	assert.True(t, ok)
	assert.Len(t, parsedError.Dir, 1)
	assert.Contains(t, parsedError.Dir[0], "hl3-release")
}

func TestTerragruntExcludeExternalDependencies(t *testing.T) {
	t.Parallel()

	excludedModule := "module-a"
	includedModule := "module-b"

	modules := []string{
		excludedModule,
		includedModule,
	}

	helpers.CleanupTerraformFolder(t, testFixtureExternalDependence)

	for _, module := range modules {
		helpers.CleanupTerraformFolder(t, util.JoinPath(testFixtureExternalDependence, module))
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	rootPath := helpers.CopyEnvironment(t, testFixtureExternalDependence)
	modulePath := util.JoinPath(rootPath, testFixtureExternalDependence, includedModule)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt run --all apply --non-interactive --queue-exclude-external --tf-forward-stdout --working-dir "+modulePath,
		&applyAllStdout,
		&applyAllStderr,
	)
	helpers.LogBufferContentsLineByLine(t, applyAllStdout, "run --all apply stdout")
	helpers.LogBufferContentsLineByLine(t, applyAllStderr, "run --all apply stderr")

	applyAllStdoutString := applyAllStdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Contains(t, applyAllStdoutString, "Hello World, "+includedModule)
	assert.NotContains(t, applyAllStdoutString, "Hello World, "+excludedModule)
}

func TestApplySkipTrue(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSkipLegacyRoot)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureSkipLegacyRoot, "skip-true")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --log-level info --non-interactive --working-dir %s --var person=Hobbs", rootPath), &showStdout, &showStderr)
	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	stdout := showStdout.String()
	stderr := showStderr.String()

	require.NoError(t, err)
	// For single unit execution, early exit message should appear
	output := stderr + stdout
	assert.Contains(t, output, "Early exit in terragrunt unit")
	assert.Contains(t, output, "due to exclude block with no_run = true")
	assert.NotContains(t, stdout, "hello, Hobbs")
}

func TestApplySkipFalse(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureSkipLegacyRoot)
	rootPath = util.JoinPath(rootPath, testFixtureSkipLegacyRoot, "skip-false")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --tf-forward-stdout --working-dir "+rootPath, &showStdout, &showStderr)
	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	stderr := showStderr.String()
	stdout := showStdout.String()

	require.NoError(t, err)
	assert.Contains(t, stdout, "hello, Hobbs")
	assert.NotContains(t, stderr, "Early exit in terragrunt unit")
}

func TestApplyAllSkipTrue(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureSkip)
	rootPath = util.JoinPath(rootPath, testFixtureSkip, "skip-true")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt run --all apply --non-interactive --tf-forward-stdout --working-dir %s --log-level info", rootPath), &showStdout, &showStderr)
	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	stdout := showStdout.String()
	stderr := showStderr.String()

	// this test is now prepared to handle the case where skip is inherited from the included terragrunt file
	// meaning the skip-true/resource2 module will be skipped as well and only the skip-true/resource1 module will be applied

	require.NoError(t, err)
	// Check that units were excluded at stack level (shown in Run Summary)
	output := stderr + stdout
	assert.Contains(t, output, "Excluded")
	assert.Contains(t, stdout, "hello, Ernie")
	assert.NotContains(t, stdout, "hello, Bert")
}

func TestApplyAllSkipFalse(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureSkip)
	rootPath = util.JoinPath(rootPath, testFixtureSkip, "skip-false")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --tf-forward-stdout --working-dir "+rootPath, &showStdout, &showStderr)
	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	stdout := showStdout.String()
	stderr := showStderr.String()

	require.NoError(t, err)
	assert.Contains(t, stdout, "hello, Ernie")
	assert.Contains(t, stdout, "hello, Bert")
	assert.NotContains(t, stderr, "Early exit in terragrunt unit")
}

func TestDependencyOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "integration")

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	// verify expected output 42
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	app3Path := util.JoinPath(rootPath, "app3")
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+app3Path, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, 42, int(outputs["z"].Value.(float64)))
}

func TestDependencyOutputErrorBeforeApply(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "integration")
	app3Path := filepath.Join(rootPath, "app3")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --working-dir "+app3Path, &showStdout, &showStderr)
	require.Error(t, err)
	// Verify that we fail because the dependency is not applied yet
	assert.Contains(t, err.Error(), "has not been applied yet")

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestDependencyOutputSkipOutputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "integration")
	emptyPath := filepath.Join(rootPath, "empty")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	// Test that even if the dependency (app1) is not applied, using skip_outputs will skip pulling the outputs so there
	// will be no errors.
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --working-dir "+emptyPath, &showStdout, &showStderr)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestDependencyOutputSkipOutputsWithMockOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "mock-outputs")
	dependent3Path := filepath.Join(rootPath, "dependent3")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+dependent3Path, &showStdout, &showStderr)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+dependent3Path, &stdout, &stderr),
	)
	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 0", outputs["truth"].Value)

	// Now run --all apply so that the dependency is applied, and verify it still uses the mock output
	err = helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath, &showStdout, &showStderr)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+dependent3Path, &stdout, &stderr),
	)
	outputs = map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 0", outputs["truth"].Value)
}

// Test that when you have a mock_output on a dependency, the dependency will use the mock as the output instead
// of erroring out.
func TestDependencyMockOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "mock-outputs")
	dependent1Path := filepath.Join(rootPath, "dependent1")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+dependent1Path, &showStdout, &showStderr)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+dependent1Path, &stdout, &stderr),
	)
	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 0", outputs["truth"].Value)

	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// Now run --all apply so that the dependency is applied, and verify it uses the dependency output
	err = helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath, &showStdout, &showStderr)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+dependent1Path, &stdout, &stderr),
	)
	outputs = map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 42", outputs["truth"].Value)
}

// Test default behavior when mock_outputs_merge_with_state is not set. It should behave, as before this parameter was added
// It will fail on any command if the parent state is not applied, because the state of the parent exists and it already has an output
// but not the newly added output.
func TestDependencyMockOutputMergeWithStateDefault(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-with-state", "merge-with-state-default", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --working-dir "+parentPath, &stdout, &stderr)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "plan stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify we have the default behavior if mock_outputs_merge_with_state is not set
	stdout.Reset()
	stderr.Reset()
	err = helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.Error(t, err)
	// Verify that we fail because the dependency is not applied yet, and the new attribute is not available and in
	// this case, mocked outputs are not used.
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output2\"")

	helpers.LogBufferContentsLineByLine(t, stdout, "plan stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "plan stderr")
}

// Test when mock_outputs_merge_with_state is explicitly set to false. It should behave, as before this parameter was added
// It will fail on any command if the parent state is not applied, because the state of the parent exists and it already has an output
// but not the newly added output.
func TestDependencyMockOutputMergeWithStateFalse(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-with-state", "merge-with-state-false", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --working-dir "+parentPath, &stdout, &stderr)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "plan stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify we have the default behavior if mock_outputs_merge_with_state is set to false
	stdout.Reset()
	stderr.Reset()
	err = helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.Error(t, err)
	// Verify that we fail because the dependency is not applied yet, and the new attribute is not available and in
	// this case, mocked outputs are not used.
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output2\"")

	helpers.LogBufferContentsLineByLine(t, stdout, "plan stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "plan stderr")
}

// Test when mock_outputs_merge_with_state is explicitly set to true.
// It will mock the newly added output from the parent as it was not already applied to the state.
func TestDependencyMockOutputMergeWithStateTrue(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-with-state", "merge-with-state-true", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --working-dir "+parentPath, &stdout, &stderr)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "plan stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify mocked outputs are used if mock_outputs_merge_with_state is set to true and some output in the parent are not applied yet.
	stdout.Reset()
	stderr.Reset()
	err = helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")
	// Now check the outputs to make sure they are as expected
	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "fake-data2", outputs["test_output2_from_parent"].Value)

	helpers.LogBufferContentsLineByLine(t, stdout, "output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "output stderr")
}

// Test when mock_outputs_merge_with_state is explicitly set to true, but using an unallowed command. It should ignore
// the mock output.
func TestDependencyMockOutputMergeWithStateTrueNotAllowed(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-with-state", "merge-with-state-true-validate-only", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --working-dir "+parentPath, &stdout, &stderr)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "plan stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify mocked outputs are used if mock_outputs_merge_with_state is set to true with an allowed command and some
	// output in the parent are not applied yet.
	stdout.Reset()
	stderr.Reset()
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt validate --non-interactive --working-dir "+childPath, &stdout, &stderr),
	)

	// ... but not when an unallowed command is used
	require.Error(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath, &stdout, &stderr),
	)
}

// Test when mock_outputs_merge_with_state is explicitly set to true.
// Mock should not be used as the parent state was already fully applied.
func TestDependencyMockOutputMergeWithStateNoOverride(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-with-state", "merge-with-state-no-override", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --working-dir "+parentPath, &stdout, &stderr)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "show stderr")

	// Verify mocked outputs are not used if mock_outputs_merge_with_state is set to true and all outputs in the parent have been applied.
	stdout.Reset()
	stderr.Reset()
	err = helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)

	// Now check the outputs to make sure they are as expected
	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "value2", outputs["test_output2_from_parent"].Value)

	helpers.LogBufferContentsLineByLine(t, stdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "show stderr")
}

// Test when mock_outputs_merge_strategy_with_state or mock_outputs_merge_with_state is not set, the default is no_merge
func TestDependencyMockOutputMergeStrategyWithStateDefault(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-default", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output_list_string\"")
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_with_state = "false" that MergeStrategyType is set to no_merge
func TestDependencyMockOutputMergeStrategyWithStateCompatFalse(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-compat-false", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output_list_string\"")
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_with_state = "true" that MergeStrategyType is set to shallow
func TestDependencyMockOutputMergeStrategyWithStateCompatTrue(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-compat-true", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath, &stdout, &stderr))
	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	helpers.LogBufferContentsLineByLine(t, stdout, "output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "fake-list-data", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when both mock_outputs_merge_with_state and mock_outputs_merge_strategy_with_state are set, mock_outputs_merge_strategy_with_state is used
func TestDependencyMockOutputMergeStrategyWithStateCompatConflict(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-compat-true", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath, &stdout, &stderr))
	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	helpers.LogBufferContentsLineByLine(t, stdout, "output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "fake-list-data", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when mock_outputs_merge_strategy_with_state = "no_merge" that mocks are not merged into the current state
func TestDependencyMockOutputMergeStrategyWithStateNoMerge(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-no-merge", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output_list_string\"")
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_strategy_with_state = "shallow" that only top level outputs are merged.
// Lists or keys in existing maps will not be merged
func TestDependencyMockOutputMergeStrategyWithStateShallow(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-shallow", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath, &stdout, &stderr))
	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	helpers.LogBufferContentsLineByLine(t, stdout, "output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "fake-list-data", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when mock_outputs_merge_strategy_with_state = "deep" that the existing state is deeply merged into the mocks
// so that the existing state overwrites the mocks. This allows child modules to use new dependency outputs before the
// dependency has been applied
func TestDependencyMockOutputMergeStrategyWithStateDeepMapOnly(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-deep-map-only", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+childPath, &stdout, &stderr))
	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	helpers.LogBufferContentsLineByLine(t, stdout, "output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "fake-abc", outputs["test_output2_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Equal(t, "fake-abc", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "a", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test that when you have a mock_output on a dependency, the dependency will use the mock as the output instead
// of erroring out when running an allowed command.
func TestDependencyMockOutputRestricted(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "mock-outputs")
	dependent2Path := filepath.Join(rootPath, "dependent2")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+dependent2Path, &showStdout, &showStderr)
	require.Error(t, err)
	// Verify that we fail because the dependency is not applied yet
	assert.Contains(t, err.Error(), "has not been applied yet")

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// Verify we can run when using one of the allowed commands
	showStdout.Reset()
	showStderr.Reset()
	err = helpers.RunTerragruntCommand(t, "terragrunt validate --non-interactive --working-dir "+dependent2Path, &showStdout, &showStderr)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// Verify that run --all validate works as well.
	showStdout.Reset()
	showStderr.Reset()
	err = helpers.RunTerragruntCommand(t, "terragrunt run --all validate --non-interactive --working-dir "+rootPath, &showStdout, &showStderr)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	showStdout.Reset()
	showStderr.Reset()
	err = helpers.RunTerragruntCommand(t, "terragrunt run --all validate --non-interactive --working-dir "+rootPath, &showStdout, &showStderr)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestDependencyOutputTypeConversion(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, ".")

	inputsPath := util.JoinPath(tmpEnvPath, testFixtureInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "type-conversion")

	// First apply the inputs module
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+inputsPath)

	// Then apply the outputs module
	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath, &showStdout, &showStderr),
	)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, true, outputs["bool"].Value)
	assert.Equal(t, []any{true, false}, outputs["list_bool"].Value)
	assert.Equal(t, []any{1.0, 2.0, 3.0}, outputs["list_number"].Value)
	assert.Equal(t, []any{"a", "b", "c"}, outputs["list_string"].Value)
	assert.Equal(t, map[string]any{"foo": true, "bar": false, "baz": true}, outputs["map_bool"].Value)
	assert.Equal(t, map[string]any{"foo": 42.0, "bar": 12345.0}, outputs["map_number"].Value)
	assert.Equal(t, map[string]any{"foo": "bar"}, outputs["map_string"].Value)
	assert.InEpsilon(t, 42.0, outputs["number"].Value.(float64), 0.0000001)
	assert.Equal(t, map[string]any{"list": []any{1.0, 2.0, 3.0}, "map": map[string]any{"foo": "bar"}, "num": 42.0, "str": "string"}, outputs["object"].Value)
	assert.Equal(t, "string", outputs["string"].Value)
	assert.Equal(t, "default", outputs["from_env"].Value)
}

// Regression testing for https://github.com/gruntwork-io/terragrunt/issues/1102: Ordering keys from
// maps to avoid random placements when terraform file is generated.
func TestOrderedMapOutputRegressions1102(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureGetOutput, "regression-1102")

	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	command := "terragrunt apply --non-interactive --working-dir " + generateTestCase
	path := filepath.Join(generateTestCase, "backend.tf")

	// runs terragrunt for the first time and checks the output "backend.tf" file.
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, command, &stdout, &stderr),
	)

	expected, _ := os.ReadFile(path)
	assert.Contains(t, string(expected), "local")

	// runs terragrunt again. All the outputs must be
	// equal to the first run.
	for range 20 {
		require.NoError(
			t,
			helpers.RunTerragruntCommand(t, command, &stdout, &stderr),
		)

		actual, _ := os.ReadFile(path)
		assert.Equal(t, expected, actual)
	}
}

// Test that we get the expected error message about dependency cycles when there is a cycle in the dependency chain
func TestDependencyOutputCycleHandling(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)

	testCases := []string{
		"aa",
		"aba",
		"abca",
		"abcda",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "cycle", tc)
			fooPath := util.JoinPath(rootPath, "foo")

			planStdout := bytes.Buffer{}
			planStderr := bytes.Buffer{}
			err := helpers.RunTerragruntCommand(
				t,
				"terragrunt plan --non-interactive --working-dir "+fooPath,
				&planStdout,
				&planStderr,
			)
			helpers.LogBufferContentsLineByLine(t, planStdout, "plan stdout")
			helpers.LogBufferContentsLineByLine(t, planStderr, "plan stderr")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "Found a dependency cycle between modules")
		})
	}
}

// Regression testing for https://github.com/gruntwork-io/terragrunt/issues/854: Referencing a dependency that is a
// subdirectory of the current config, which includes an `include` block has problems resolving the correct relative
// path.
func TestDependencyOutputRegression854(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "regression-854", "root")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

// Regression testing for bug where terragrunt output runs on dependency blocks are done in the terragrunt-cache for the
// child, not the parent.
func TestDependencyOutputCachePathBug(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "localstate", "live")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

func TestDependencyOutputWithTerragruntSource(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "regression-1124", "live")
	modulePath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "regression-1124", "modules")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		fmt.Sprintf("terragrunt run --all apply --non-interactive --working-dir %s --source %s", rootPath, modulePath),
		&stdout,
		&stderr,
	)
	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

func TestDependencyOutputWithHooks(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "regression-1273")
	depPathFileOut := util.JoinPath(rootPath, "dep", "file.out")
	mainPath := util.JoinPath(rootPath, "main")
	mainPathFileOut := util.JoinPath(mainPath, "file.out")

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)
	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// The file should exist in the first run.
	assert.True(t, util.FileExists(depPathFileOut))
	assert.False(t, util.FileExists(mainPathFileOut))

	// Now delete file and run plain main again. It should NOT create file.out.
	require.NoError(t, os.Remove(depPathFileOut))
	helpers.RunTerragrunt(t, "terragrunt plan --non-interactive --working-dir "+mainPath)
	assert.False(t, util.FileExists(depPathFileOut))
	assert.False(t, util.FileExists(mainPathFileOut))
}

func TestDeepDependencyOutputWithMock(t *testing.T) {
	// Test that the terraform command flows through for mock output retrieval to deeper dependencies. Previously the
	// terraform command was being overwritten, so by the time the deep dependency retrieval runs, it was replaced with
	// "output" instead of the original one.
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "nested-mocks", "live")

	// Since we haven't applied anything, this should only succeed if mock outputs are used.
	helpers.RunTerragrunt(t, "terragrunt validate --non-interactive --working-dir "+rootPath)
}

func TestDataDir(t *testing.T) {
	// Cannot be run in parallel with other tests as it modifies process' environment.
	helpers.CleanupTerraformFolder(t, testFixtureDirsPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDirsPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDirsPath)

	t.Setenv("TF_DATA_DIR", util.JoinPath(tmpEnvPath, "data_dir"))

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Initializing provider plugins")

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)
	assert.NotContains(t, stdout.String(), "Initializing provider plugins")
}

func TestReadTerragruntConfigWithDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureReadConfig)
	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, ".")

	inputsPath := util.JoinPath(tmpEnvPath, testFixtureInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReadConfig, "with_dependency")

	// First apply the inputs module
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+inputsPath)

	// Then apply the read config module
	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath, &showStdout, &showStderr),
	)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, true, outputs["bool"].Value)
	assert.Equal(t, []any{true, false}, outputs["list_bool"].Value)
	assert.Equal(t, []any{1.0, 2.0, 3.0}, outputs["list_number"].Value)
	assert.Equal(t, []any{"a", "b", "c"}, outputs["list_string"].Value)
	assert.Equal(t, map[string]any{"foo": true, "bar": false, "baz": true}, outputs["map_bool"].Value)
	assert.Equal(t, map[string]any{"foo": 42.0, "bar": 12345.0}, outputs["map_number"].Value)
	assert.Equal(t, map[string]any{"foo": "bar"}, outputs["map_string"].Value)
	assert.InEpsilon(t, 42.0, outputs["number"].Value.(float64), 0.0000001)
	assert.Equal(t, map[string]any{"list": []any{1.0, 2.0, 3.0}, "map": map[string]any{"foo": "bar"}, "num": 42.0, "str": "string"}, outputs["object"].Value)
	assert.Equal(t, "string", outputs["string"].Value)
	assert.Equal(t, "default", outputs["from_env"].Value)
}

func TestReadTerragruntConfigFromDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureReadConfig)
	tmpEnvPath := helpers.CopyEnvironment(t, ".")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReadConfig, "from_dependency")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath, &showStdout, &showStderr),
	)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "hello world", outputs["bar"].Value)
}

func TestReadTerragruntConfigWithDefault(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureReadConfig)
	rootPath := util.JoinPath(testFixtureReadConfig, "with_default")

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	// check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "default value", outputs["data"].Value)
}

func TestReadTerragruntConfigWithOriginalTerragruntDir(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureReadConfig)
	rootPath := util.JoinPath(testFixtureReadConfig, "with_original_terragrunt_dir")

	rootPathAbs, err := filepath.Abs(rootPath)
	require.NoError(t, err)

	fooPathAbs := filepath.Join(rootPathAbs, "foo")
	depPathAbs := filepath.Join(rootPathAbs, "dep")

	// Run apply on the dependency module and make sure we get the outputs we expect
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+depPathAbs)

	depStdout := bytes.Buffer{}
	depStderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+depPathAbs, &depStdout, &depStderr),
	)

	depOutputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(depStdout.Bytes(), &depOutputs))

	assert.Equal(t, depPathAbs, depOutputs["terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, depOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, depOutputs["bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, depOutputs["bar_original_terragrunt_dir"].Value)

	// Run apply on the root module and make sure we get the expected outputs
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	rootStdout := bytes.Buffer{}
	rootStderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &rootStdout, &rootStderr),
	)

	rootOutputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(rootStdout.Bytes(), &rootOutputs))

	assert.Equal(t, fooPathAbs, rootOutputs["terragrunt_dir"].Value)
	assert.Equal(t, rootPathAbs, rootOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, rootOutputs["dep_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, rootOutputs["dep_original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, rootOutputs["dep_bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, rootOutputs["dep_bar_original_terragrunt_dir"].Value)

	// Run 'run --all apply' and make sure all the outputs are identical in the root module and the dependency module
	helpers.RunTerragrunt(t, "terragrunt run --all  --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")

	runAllRootStdout := bytes.Buffer{}
	runAllRootStderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &runAllRootStdout, &runAllRootStderr),
	)

	runAllRootOutputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(runAllRootStdout.Bytes(), &runAllRootOutputs))

	runAllDepStdout := bytes.Buffer{}
	runAllDepStderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+depPathAbs, &runAllDepStdout, &runAllDepStderr),
	)

	runAllDepOutputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(runAllDepStdout.Bytes(), &runAllDepOutputs))

	assert.Equal(t, fooPathAbs, runAllRootOutputs["terragrunt_dir"].Value)
	assert.Equal(t, rootPathAbs, runAllRootOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllRootOutputs["dep_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllRootOutputs["dep_original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, runAllRootOutputs["dep_bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllRootOutputs["dep_bar_original_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllDepOutputs["terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllDepOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, runAllDepOutputs["bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllDepOutputs["bar_original_terragrunt_dir"].Value)
}

func TestReadTerragruntConfigFull(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureReadConfig)
	rootPath := util.JoinPath(testFixtureReadConfig, "full")

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)

	// check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "terragrunt", outputs["terraform_binary"].Value)
	assert.Equal(t, "= 0.12.20", outputs["terraform_version_constraint"].Value)
	assert.Equal(t, "= 0.23.18", outputs["terragrunt_version_constraint"].Value)
	assert.Equal(t, ".terragrunt-cache", outputs["download_dir"].Value)
	assert.Equal(t, "TerragruntIAMRole", outputs["iam_role"].Value)
	// exclude is now a block, not a simple boolean - just verify it exists
	assert.Contains(t, outputs, "exclude")
	assert.NotEmpty(t, outputs["exclude"].Value)
	assert.Equal(t, "true", outputs["prevent_destroy"].Value)

	// Simple maps
	localstgOut := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(outputs["localstg"].Value.(string)), &localstgOut))
	assert.Equal(t, map[string]any{"the_answer": float64(42)}, localstgOut)
	inputsOut := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(outputs["inputs"].Value.(string)), &inputsOut))
	assert.Equal(t, map[string]any{"doc": "Emmett Brown"}, inputsOut)

	// Complex blocks
	depsOut := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(outputs["dependencies"].Value.(string)), &depsOut))
	assert.Equal(
		t,
		map[string]any{
			"paths": []any{"../../terragrunt"},
		},
		depsOut,
	)

	generateOut := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(outputs["generate"].Value.(string)), &generateOut))
	assert.Equal(
		t,
		map[string]any{
			"provider": map[string]any{
				"path":              "provider.tf",
				"if_exists":         "overwrite_terragrunt",
				"hcl_fmt":           nil,
				"if_disabled":       "skip",
				"comment_prefix":    "# ",
				"disable_signature": false,
				"disable":           false,
				"contents": `provider "aws" {
  region = "us-east-1"
}
`,
			},
		},
		generateOut,
	)
	remoteStateOut := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(outputs["remote_state"].Value.(string)), &remoteStateOut))
	assert.Equal(
		t,
		map[string]any{
			"backend":                         "local",
			"disable_init":                    false,
			"disable_dependency_optimization": false,
			"generate":                        map[string]any{"path": "backend.tf", "if_exists": "overwrite_terragrunt"},
			"config":                          map[string]any{"path": "foo.tfstate"},
			"encryption":                      map[string]any{"key_provider": "foo"},
		},
		remoteStateOut,
	)
	terraformOut := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(outputs["terraformtg"].Value.(string)), &terraformOut))
	assert.Equal(
		t,
		map[string]any{
			"source":                   "./delorean",
			"include_in_copy":          []any{"time_machine.*"},
			"exclude_from_copy":        []any{"excluded_time_machine.*"},
			"copy_terraform_lock_file": true,
			"extra_arguments": map[string]any{
				"var-files": map[string]any{
					"name":               "var-files",
					"commands":           []any{"apply", "plan"},
					"arguments":          nil,
					"required_var_files": []any{"extra.tfvars"},
					"optional_var_files": []any{"optional.tfvars"},
					"env_vars": map[string]any{
						"TF_VAR_custom_var": "I'm set in extra_arguments env_vars",
					},
				},
			},
			"before_hook": map[string]any{
				"before_hook_1": map[string]any{
					"name":            "before_hook_1",
					"commands":        []any{"apply", "plan"},
					"execute":         []any{"touch", "before.out"},
					"working_dir":     nil,
					"run_on_error":    true,
					"if":              nil,
					"suppress_stdout": nil,
				},
			},
			"after_hook": map[string]any{
				"after_hook_1": map[string]any{
					"name":            "after_hook_1",
					"commands":        []any{"apply", "plan"},
					"execute":         []any{"touch", "after.out"},
					"working_dir":     nil,
					"run_on_error":    true,
					"if":              nil,
					"suppress_stdout": nil,
				},
			},
			"error_hook": map[string]any{},
		},
		terraformOut,
	)
}

func TestTerragruntGenerateBlockSkipRemove(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := util.JoinPath(tmpEnvPath, testFixtureCodegenPath, "remove-file", "skip")

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	assert.FileExists(t, filepath.Join(generateTestCase, "backend.tf"))
}

func TestTerragruntGenerateBlockRemove(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := util.JoinPath(tmpEnvPath, testFixtureCodegenPath, "remove-file", "remove")

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	assert.NoFileExists(t, filepath.Join(generateTestCase, "backend.tf"))
}

func TestTerragruntGenerateBlockRemoveTerragruntSuccess(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := util.JoinPath(tmpEnvPath, testFixtureCodegenPath, "remove-file", "remove_terragrunt")

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	assert.NoFileExists(t, filepath.Join(generateTestCase, "backend.tf"))
}

func TestTerragruntGenerateBlockRemoveTerragruntFail(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := util.JoinPath(tmpEnvPath, testFixtureCodegenPath, "remove-file", "remove_terragrunt_error")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	require.Error(t, err)

	var generateFileRemoveError codegen.GenerateFileRemoveError

	ok := errors.As(err, &generateFileRemoveError)
	assert.True(t, ok)

	assert.FileExists(t, filepath.Join(generateTestCase, "backend.tf"))
}

func TestTerragruntGenerateBlockSkip(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "skip")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	assert.False(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
}

func TestTerragruntGenerateBlockOverwrite(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "overwrite")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, helpers.FileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTerragruntGenerateAttr(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-attr")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	text := "test-terragrunt-generate-attr-hello-world"

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --tf-forward-stdout --working-dir %s -var text=\"%s\"", generateTestCase, text))
	require.NoError(t, err)
	assert.Contains(t, stdout, text)
}

func TestTerragruntGenerateBlockOverwriteTerragruntSuccess(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "overwrite_terragrunt")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, helpers.FileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTerragruntGenerateBlockOverwriteTerragruntFail(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "overwrite_terragrunt_error")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)

	var generateFileExistsError codegen.GenerateFileExistsError

	ok := errors.As(err, &generateFileExistsError)
	assert.True(t, ok)
}

func TestTerragruntGenerateBlockNestedInherit(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "nested", "child_inherit")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	// If the state file was written as foo.tfstate, that means it inherited the config
	assert.True(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, helpers.FileIsInFolder(t, "bar.tfstate", generateTestCase))
	// Also check to make sure the child config generate block was included
	assert.True(t, helpers.FileIsInFolder(t, "random_file.txt", generateTestCase))
}

func TestTerragruntGenerateBlockNestedOverwrite(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "nested", "child_overwrite")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	// If the state file was written as bar.tfstate, that means it overwrite the parent config
	assert.False(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.True(t, helpers.FileIsInFolder(t, "bar.tfstate", generateTestCase))
	// Also check to make sure the child config generate block was included
	assert.True(t, helpers.FileIsInFolder(t, "random_file.txt", generateTestCase))
}

func TestTerragruntGenerateBlockDisableSignature(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "disable-signature")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+generateTestCase, &stdout, &stderr),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "Hello, World!", outputs["text"].Value)
}

func TestTerragruntGenerateBlockSameNameFail(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "same_name_error")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt init --working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)

	var parsedError config.DuplicatedGenerateBlocksError

	ok := errors.As(err, &parsedError)
	assert.True(t, ok)
	assert.Len(t, parsedError.BlockName, 1)
	assert.Contains(t, parsedError.BlockName, "backend")
}

func TestTerragruntGenerateBlockSameNameIncludeFail(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "same_name_includes_error")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt init --working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)

	var parsedError config.DuplicatedGenerateBlocksError

	ok := errors.As(err, &parsedError)
	assert.True(t, ok)
	assert.Len(t, parsedError.BlockName, 1)
	assert.Contains(t, parsedError.BlockName, "backend")
}

func TestTerragruntGenerateBlockMultipleSameNameFail(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "same_name_pair_error")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt init --working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)

	var parsedError config.DuplicatedGenerateBlocksError

	ok := errors.As(err, &parsedError)
	assert.True(t, ok)
	assert.Len(t, parsedError.BlockName, 2)
	assert.Contains(t, parsedError.BlockName, "backend")
	assert.Contains(t, parsedError.BlockName, "backend2")
}

func TestTerragruntGenerateBlockDisable(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "disable")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt init --working-dir "+generateTestCase, &stdout, &stderr)
	require.NoError(t, err)
	assert.False(t, helpers.FileIsInFolder(t, "data.txt", generateTestCase))
}

func TestTerragruntGenerateBlockEnable(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "enable")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt init --working-dir "+generateTestCase, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, helpers.FileIsInFolder(t, "data.txt", generateTestCase))
}

func TestTerragruntRemoteStateCodegenGeneratesBackendBlock(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "remote-state", "base")

	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	// If the state file was written as foo.tfstate, that means it wrote out the local backend config.
	assert.True(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
}

func TestTerragruntRemoteStateCodegenOverwrites(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "remote-state", "overwrite")

	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, helpers.FileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTerragruntRemoteStateCodegenErrorsIfExists(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "remote-state", "error")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)

	var generateFileExistsError codegen.GenerateFileExistsError

	ok := errors.As(err, &generateFileExistsError)
	assert.True(t, ok)
}

func TestTerragruntRemoteStateCodegenDoesNotGenerateWithSkip(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "remote-state", "skip")

	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase)
	assert.False(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
}

// This function cannot be parallelized as it changes the global version.Version
//
//nolint:paralleltest
func TestTerragruntValidateAllWithVersionChecks(t *testing.T) {
	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/version-check")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntVersionCommand(t, "v0.23.21", "terragrunt run --all validate --non-interactive --working-dir "+tmpEnvPath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

func TestTerragruntIncludeParentHclFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureIncludeParent)
	tmpEnvPath = path.Join(tmpEnvPath, testFixtureIncludeParent)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --log-level debug --all apply --non-interactive --working-dir "+tmpEnvPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "parent_hcl_file")
	assert.Contains(t, stderr, "dependency_hcl")
	assert.Contains(t, stderr, "common_hcl")
}

// The tests here cannot be parallelized.
// This is due to a race condition brought about by overriding `version.Version` in
// runTerragruntVersionCommand
//
//nolint:paralleltest,funlen
func TestTerragruntVersionConstraints(t *testing.T) {
	testCases := []struct {
		name                 string
		terragruntVersion    string
		terragruntConstraint string
		shouldSucceed        bool
	}{
		{
			"version meets constraint equal",
			"v0.23.18",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			true,
		},
		{
			"version meets constraint greater patch",
			"v0.23.19",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			true,
		},
		{
			"version meets constraint greater major",
			"v1.0.0",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			true,
		},
		{
			"version fails constraint less patch",
			"v0.23.17",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			false,
		},
		{
			"version fails constraint less major",
			"v0.22.18",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			false,
		},
		{
			"version meets constraint pre-release",
			"v0.23.18-alpha2024091301",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			true,
		},
		{
			"version fails constraint pre-release",
			"v0.23.18-alpha2024091301",
			"terragrunt_version_constraint = \"< v0.23.18\"",
			false,
		},
	}

	for _, tc := range testCases { //nolint:paralleltest
		t.Run(tc.name, func(t *testing.T) {
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReadConfig)
			rootPath := filepath.Join(tmpEnvPath, testFixtureReadConfig, "with_constraints")

			tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfigContent(t, tc.terragruntConstraint, config.DefaultTerragruntConfigPath)

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}

			err := helpers.RunTerragruntVersionCommand(
				t,
				tc.terragruntVersion,
				fmt.Sprintf(
					"terragrunt apply -auto-approve --non-interactive --config %s --working-dir %s",
					tmpTerragruntConfigPath,
					rootPath,
				),
				&stdout,
				&stderr,
			)

			helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
			helpers.LogBufferContentsLineByLine(t, stderr, "stderr")

			if tc.shouldSucceed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestReadTerragruntAuthProviderCmd(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "multiple-apps")
	appPath := util.JoinPath(rootPath, "app1")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	helpers.RunTerragrunt(t, fmt.Sprintf(`terragrunt run --all --non-interactive --working-dir %s --auth-provider-cmd %s -- apply -auto-approve`, rootPath, mockAuthCmd))

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output -json --working-dir %s --auth-provider-cmd %s", appPath, mockAuthCmd))
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "app1-bar", outputs["foo-app1"].Value)
	assert.Equal(t, "app2-bar", outputs["foo-app2"].Value)
	assert.Equal(t, "app3-bar", outputs["foo-app3"].Value)
}

func TestIamRolesLoadingFromDifferentModules(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureIamRolesMultipleModules)

	// Execution outputs to be verified
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	// Invoke terragrunt and verify used IAM roles for each dependency
	err := helpers.RunTerragruntCommand(t, "terragrunt init --log-level trace --debugreset --working-dir "+testFixtureIamRolesMultipleModules, &stdout, &stderr)

	// Taking all outputs in one string
	output := fmt.Sprintf("%v %v %v", stderr.String(), stdout.String(), err.Error())

	component1 := ""
	component2 := ""

	// scan each output line and get lines for component1 and component2
	for line := range strings.SplitSeq(output, "\n") {
		if strings.Contains(line, "Assuming IAM role arn:aws:iam::component1:role/terragrunt") {
			component1 = line
			continue
		}

		if strings.Contains(line, "Assuming IAM role arn:aws:iam::component2:role/terragrunt") {
			component2 = line
			continue
		}
	}

	assert.NotEmptyf(t, component1, "Missing role for component 1")
	assert.NotEmptyf(t, component2, "Missing role for component 2")
}

// This function cannot be parallelized as it changes the global version.Version
//
//nolint:paralleltest
func TestTerragruntVersionConstraintsPartialParse(t *testing.T) {
	fixturePath := "fixtures/partial-parse/terragrunt-version-constraint"
	helpers.CleanupTerragruntFolder(t, fixturePath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntVersionCommand(t, "0.21.23", "terragrunt apply -auto-approve --non-interactive --working-dir "+fixturePath, &stdout, &stderr)
	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")

	require.Error(t, err)

	var invalidVersionError run.InvalidTerragruntVersion

	ok := errors.As(err, &invalidVersionError)
	assert.True(t, ok)
}

func TestLogFailingDependencies(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testFixtureBrokenDependency, "app")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --working-dir %s --log-level trace", path))
	require.Error(t, err)

	// Check that the error output contains terraform/tofu error details
	assert.Contains(t, stderr, "Getting output of dependency ../dependency/terragrunt.hcl")
	assert.Contains(t, stderr, "Error: Failed to download module")
}

func TestDependencyInputsBlockedByDefault(t *testing.T) {
	t.Parallel()

	// Test that using dependency.foo.inputs syntax results in an error by default
	tmpDir := t.TempDir()

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
	require.NoError(t, os.WriteFile(configPath, []byte(dependencyConfig), 0644))

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

func TestDependenciesOptimisation(t *testing.T) {
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependenciesOptimisation)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDependenciesOptimisation)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --log-level trace --working-dir "+rootPath+" -- apply -auto-approve")
	require.NoError(t, err)

	assert.NotContains( // Check that we're getting a warning for usage of deprecated functionality.
		t,
		stderr,
		"Reading inputs from dependencies has been deprecated and will be removed in a future version of Terragrunt. If a value in a dependency is needed, use dependency outputs instead.",
	)

	config.ClearOutputCache()

	moduleC := util.JoinPath(tmpEnvPath, testFixtureDependenciesOptimisation, "module-c")

	t.Setenv("TERRAGRUNT_STRICT_CONTROL", "skip-dependencies-inputs")
	_, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --log-level trace --working-dir "+moduleC)
	require.NoError(t, err)

	// checking that dependencies optimisation is working and outputs from module-a are not retrieved
	assert.NotContains(t, stderr, "Retrieved output from ../module-a/terragrunt.hcl")
}

func cleanupTerraformFolder(t *testing.T, templatesPath string) {
	t.Helper()

	removeFile(t, util.JoinPath(templatesPath, terraformState))
	removeFile(t, util.JoinPath(templatesPath, terraformStateBackup))
	removeFolder(t, util.JoinPath(templatesPath, terraformFolder))
}

func removeFile(t *testing.T, path string) {
	t.Helper()

	if util.FileExists(path) {
		if err := os.Remove(path); err != nil {
			t.Fatalf("Error while removing %s: %v", path, err)
		}
	}
}

func removeFolder(t *testing.T, path string) {
	t.Helper()

	if util.FileExists(path) {
		if err := os.RemoveAll(path); err != nil {
			t.Fatalf("Error while removing %s: %v", path, err)
		}
	}
}

func TestShowErrorWhenRunAllInvokedWithoutArguments(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStack)
	appPath := util.JoinPath(tmpEnvPath, testFixtureStack)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt run --all --non-interactive --working-dir "+appPath, &stdout, &stderr)
	require.Error(t, err)

	var missingCommandError runall.MissingCommand

	ok := errors.As(err, &missingCommandError)
	assert.True(t, ok)
}

func TestNoMultipleInitsWithoutSourceChange(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDownload)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureStdout)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	// providers initialization during first plan
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	// no initialization expected for second plan run
	// https://github.com/gruntwork-io/terragrunt/issues/1921
	assert.Equal(t, 0, strings.Count(stdout.String(), "has been successfully initialized!"))
}

func TestAutoInitWhenSourceIsChanged(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDownload)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAutoInit)

	terragruntHcl := util.JoinPath(testPath, "terragrunt.hcl")

	contents, err := util.ReadFileAsString(terragruntHcl)
	if err != nil {
		require.NoError(t, err)
	}

	updatedHcl := strings.ReplaceAll(contents, "__TAG_VALUE__", "v0.78.4")
	require.NoError(t, os.WriteFile(terragruntHcl, []byte(updatedHcl), 0444))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	// providers initialization during first plan
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))

	updatedHcl = strings.ReplaceAll(contents, "__TAG_VALUE__", "v0.79.0")
	require.NoError(t, os.WriteFile(terragruntHcl, []byte(updatedHcl), 0444))

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = helpers.RunTerragruntCommand(t, "terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	// auto initialization when source is changed
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))
}

func TestNoColor(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoColor)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureNoColor)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt plan -no-color --tf-forward-stdout --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	// providers initialization during first plan
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))

	assert.NotContains(t, stdout.String(), "\x1b")
}

func TestTerragruntValidateModulePrefix(t *testing.T) {
	t.Parallel()

	fixturePath := testFixtureIncludeParent
	helpers.CleanupTerraformFolder(t, fixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, fixturePath)
	rootPath := util.JoinPath(tmpEnvPath, fixturePath)

	helpers.RunTerragrunt(t, "terragrunt run --all validate --tf-forward-stdout --non-interactive --working-dir "+rootPath)
}

func TestInitFailureModulePrefix(t *testing.T) {
	t.Parallel()

	initTestCase := testFixtureInitError

	helpers.CleanupTerraformFolder(t, initTestCase)
	helpers.CleanupTerragruntFolder(t, initTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.Error(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt init -no-color --non-interactive --working-dir "+initTestCase, &stdout, &stderr),
	)
	helpers.LogBufferContentsLineByLine(t, stderr, "init")
	// Check for terraform error in structured log format
	assert.Contains(t, stderr.String(), "level=stderr")
}

func TestDependencyOutputModulePrefix(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "integration")

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	// verify expected output 42
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	app3Path := util.JoinPath(rootPath, "app3")
	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+app3Path, &stdout, &stderr),
	)
	// validate that output is valid json
	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, 42, int(outputs["z"].Value.(float64)))
}

func TestExplainingMissingCredentials(t *testing.T) {
	// no parallel because we need to set env vars
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/tmp/not-existing-creds-46521694")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInitError)
	initTestCase := util.JoinPath(tmpEnvPath, testFixtureInitError)

	helpers.CleanupTerraformFolder(t, initTestCase)
	helpers.CleanupTerragruntFolder(t, initTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt init -no-color --tf-forward-stdout --non-interactive --working-dir "+initTestCase, &stdout, &stderr)
	explanation := shell.ExplainError(err)
	assert.Contains(t, explanation, "Missing AWS credentials")
}

func TestModulePathInPlanErrorMessage(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureModulePathError)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureModulePathError, "app")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan -no-color --non-interactive --working-dir "+rootPath)
	require.Error(t, err)
	output := stdout + "\n" + stderr + "\n" + err.Error() + "\n"

	assert.Contains(t, output, "error occurred")
}

func TestModulePathInRunAllPlanErrorMessage(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureModulePathError)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureModulePathError)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan -no-color")
	require.NoError(t, err)

	output := fmt.Sprintf("%s\n%s\n", stdout, stderr)
	// catch "Run failed" message printed in case of error in apply of units
	assert.Contains(t, output, "Run failed")
	assert.Contains(t, output, "Unit d1", output)
}

func TestHclFmtDiff(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHclfmtDiff)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclfmtDiff)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclfmtDiff)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt hcl fmt --diff --working-dir "+rootPath, &stdout, &stderr),
	)

	output := stdout.String()

	expectedDiff, err := os.ReadFile(util.JoinPath(rootPath, "expected.diff"))
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, stdout, "output")
	assert.Contains(t, output, string(expectedDiff))
}

func TestHclFmtStdin(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureHclfmtStdin)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHclfmtStdin)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclfmtStdin)

	os.Stdin, _ = os.Open(util.JoinPath(rootPath, "terragrunt.hcl"))

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt hcl fmt --stdin")
	require.NoError(t, err)

	expectedDiff, err := os.ReadFile(util.JoinPath(rootPath, "expected.hcl"))
	require.NoError(t, err)

	assert.Contains(t, stdout, string(expectedDiff))
}

func TestInitSkipCache(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInitCache)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInitCache)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureInitCache, "app")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt plan --log-level trace --non-interactive --tf-forward-stdout --working-dir "+rootPath, &stdout, &stderr),
	)

	// verify that init was invoked
	assert.Contains(t, stdout.String(), "has been successfully initialized!")
	assert.Contains(t, stderr.String(), "Running command: "+wrappedBinary()+" init")

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt plan --log-level trace --non-interactive --tf-forward-stdout --working-dir "+rootPath, &stdout, &stderr),
	)

	// verify that init wasn't invoked second time since cache directories are ignored
	assert.NotContains(t, stdout.String(), "has been successfully initialized!")
	assert.NotContains(t, stderr.String(), "Running command: "+wrappedBinary()+" init")

	// verify that after adding new file, init is executed
	tfFile := util.JoinPath(tmpEnvPath, testFixtureInitCache, "app", "project.tf")
	if err := os.WriteFile(tfFile, []byte(""), 0644); err != nil {
		t.Fatalf("Error writing new Terraform file to %s: %v", tfFile, err)
	}

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt plan --log-level trace --non-interactive --tf-forward-stdout --working-dir "+rootPath, &stdout, &stderr),
	)

	// verify that init was invoked
	assert.Contains(t, stdout.String(), "has been successfully initialized!")
	assert.Contains(t, stderr.String(), "Running command: "+wrappedBinary()+" init")
}

func TestTerragruntFailIfBucketCreationIsrequired(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)
	helpers.CleanupTerraformFolder(t, rootPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(helpers.UniqueID())

	tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfig(t, rootPath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt apply --fail-on-state-bucket-creation --non-interactive --config %s --working-dir %s", tmpTerragruntConfigPath, rootPath), &stdout, &stderr)
	require.Error(t, err)
}

func TestTerragruntPassNullValues(t *testing.T) {
	t.Parallel()

	generateTestCase := testFixtureNullValue
	tmpEnv := helpers.CopyEnvironment(t, generateTestCase)
	helpers.CleanupTerraformFolder(t, tmpEnv)
	helpers.CleanupTerragruntFolder(t, tmpEnv)
	tmpEnv = util.JoinPath(tmpEnv, generateTestCase)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+tmpEnv)

	// Now check the outputs to make sure they are as expected
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --non-interactive --working-dir "+tmpEnv)

	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	// check that the null values are passed correctly
	assert.Nil(t, outputs["output1"].Value)
	assert.Equal(t, "variable 2", outputs["output2"].Value)

	// check that file with null values is removed
	cachePath := filepath.Join(tmpEnv, helpers.TerragruntCache)
	foundNullValuesFile := false
	err = filepath.Walk(cachePath,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if strings.HasPrefix(path, run.NullTFVarsFile) {
				foundNullValuesFile = true
			}

			return nil
		})

	assert.Falsef(t, foundNullValuesFile, "Found %s file in cache directory", run.NullTFVarsFile)
	require.NoError(t, err)
}

func TestTerragruntHandleLegacyNullValues(t *testing.T) {
	// no parallel since we need to set env vars
	t.Setenv("TERRAGRUNT_TEMP_QUOTE_NULL", "1")

	generateTestCase := testFixtureNullValue
	tmpEnv := helpers.CopyEnvironment(t, generateTestCase)
	helpers.CleanupTerraformFolder(t, tmpEnv)
	helpers.CleanupTerragruntFolder(t, tmpEnv)
	tmpEnv = util.JoinPath(tmpEnv, generateTestCase)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+tmpEnv)
	require.NoError(t, err)
	assert.Contains(t, stderr, "Input `var1` has value `null`. Quoting due to TERRAGRUNT_TEMP_QUOTE_NULL")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --non-interactive --working-dir "+tmpEnv)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	// check that null value is passed as "null"
	assert.Equal(t, "null", outputs["output1"].Value)
	assert.Equal(t, "variable 2", outputs["output2"].Value)
}

func TestTerragruntNoWarningLocalPath(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDisabledPath)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureDisabledPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply --non-interactive --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	assert.NotContains(t, stderr.String(), "No double-slash (//) found in source URL")
}

func TestTerragruntDisabledDependency(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDisabledModule)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureDisabledModule, "app")

	_, output, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all plan --non-interactive --log-level trace --working-dir "+testPath)
	require.NoError(t, err)

	// check that only enabled dependencies are evaluated

	for _, path := range []string{
		util.JoinPath(tmpEnvPath, testFixtureDisabledModule, "app"),
		util.JoinPath(tmpEnvPath, testFixtureDisabledModule, "unit-without-enabled"),
		util.JoinPath(tmpEnvPath, testFixtureDisabledModule, "unit-enabled"),
	} {
		relPath, err := filepath.Rel(testPath, path)
		require.NoError(t, err)
		assert.Contains(t, output, relPath, output)
	}

	for _, path := range []string{
		util.JoinPath(tmpEnvPath, testFixtureDisabledModule, "unit-disabled"),
	} {
		relPath, err := filepath.Rel(testPath, path)
		require.NoError(t, err)
		assert.NotContains(t, output, "- Unit "+relPath, output)
	}
}

func TestTerragruntHandleEmptyStateFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureEmptyState)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureEmptyState)

	helpers.CreateEmptyStateFile(t, testPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testPath)
}

func TestTerragruntCommandsThatNeedInput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCommandsThatNeedInput)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureCommandsThatNeedInput)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply --non-interactive --tf-forward-stdout --working-dir "+testPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Apply complete")
}

func TestTerragruntSkipDependenciesWithSkipFlag(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSkipDependencies)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureSkipDependencies)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --no-color --non-interactive --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	output := fmt.Sprintf("%s %s", stderr.String(), stdout.String())

	assert.NotContains(t, output, "Error reading partial config for dependency")
	assert.NotContains(t, output, "Call to function \"find_in_parent_folders\" failed")
	assert.NotContains(t, output, "ParentFileNotFoundError")

	// Check that units were excluded at stack level (shown in Run Summary)
	assert.Contains(t, output, "Excluded")
	// check that no test_file.txt was created in module directory
	_, err = os.Stat(util.JoinPath(tmpEnvPath, testFixtureSkipDependencies, "first", "test_file.txt"))
	require.Error(t, err)
	_, err = os.Stat(util.JoinPath(tmpEnvPath, testFixtureSkipDependencies, "second", "test_file.txt"))
	require.Error(t, err)
}

func TestTerragruntInfoError(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInfoError)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureInfoError, "module-b")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt info print --non-interactive --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	// parse stdout json as InfoOutput
	var output print.InfoOutput

	err = json.Unmarshal(stdout.Bytes(), &output)
	require.NoError(t, err)
}

func TestStorePlanFilesRunAllPlanApply(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := t.TempDir()

	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)
	dependencyPath := util.JoinPath(tmpEnvPath, testFixtureOutDir, "dependency")

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --log-level trace --working-dir %s --out-dir %s", dependencyPath, tmpDir))

	// run plan with output directory
	_, output, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all plan --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	assert.Contains(t, output, "Using output file "+getPathRelativeTo(t, tmpDir, testPath))

	// verify that tfplan files are created in the tmpDir, 2 files
	list, err := findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all apply --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)
}

func TestStorePlanFilesRunAllPlanApplyRelativePath(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	dependencyPath := util.JoinPath(tmpEnvPath, testFixtureOutDir, "dependency")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --log-level trace --working-dir %s --out-dir %s", dependencyPath, testPath))

	// run plan with output directory
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all plan --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, "test"))
	require.NoError(t, err)

	outDir := util.JoinPath(testPath, "test")

	// verify that tfplan files are created in the tmpDir, 2 files
	list, err := findFilesWithExtension(outDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all apply --non-interactive --log-level trace --working-dir %s --out-dir test", testPath))
	require.NoError(t, err)
}

func TestUsingAllAndGraphFlagsSimultaneously(t *testing.T) {
	t.Parallel()

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --graph --all")
	expectedErr := new(common.AllGraphFlagsError)
	require.ErrorAs(t, err, &expectedErr)
}

func TestStorePlanFilesJsonRelativePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args string
	}{
		{"run --all plan --non-interactive --log-level trace --working-dir %s --out-dir test --json-out-dir json"},
		{"run plan --all --non-interactive --log-level trace --working-dir %s --out-dir test --json-out-dir json"},
		{"run plan -a --non-interactive --log-level trace --working-dir %s --out-dir test --json-out-dir json"},
		{"run --all --non-interactive --log-level trace --working-dir %s --out-dir test --json-out-dir json -- plan"},
	}

	for _, tc := range testCases {
		t.Run("terragrunt args: "+tc.args, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
			helpers.CleanupTerraformFolder(t, tmpEnvPath)
			testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

			// run plan with output directory
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt "+tc.args, testPath))
			require.NoError(t, err)

			// verify that tfplan files are created in the tmpDir, 2 files
			outDir := util.JoinPath(testPath, "test")
			list, err := findFilesWithExtension(outDir, ".tfplan")
			require.NoError(t, err)
			assert.Len(t, list, 2)

			// verify that json files are create
			jsonDir := util.JoinPath(testPath, "json")
			listJSON, err := findFilesWithExtension(jsonDir, ".json")
			require.NoError(t, err)
			assert.Len(t, listJSON, 2)
		})
	}
}

func TestPlanJsonFilesRunAll(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := t.TempDir()
	_, _, _, err := testRunAllPlan(t, "--json-out-dir "+tmpDir, "")
	require.NoError(t, err)

	// verify that was generated json files with plan data
	list, err := findFilesWithExtension(tmpDir, ".json")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	for _, file := range list {
		assert.Equal(t, "tfplan.json", filepath.Base(file))
		// verify that file is not empty
		content, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.NotEmpty(t, content)
		// check that produced json is valid and can be unmarshalled
		var plan map[string]any

		err = json.Unmarshal(content, &plan)
		require.NoError(t, err)
		// check that plan is not empty
		assert.NotEmpty(t, plan)
	}
}

func TestPlanJsonPlanBinaryRunAll(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := t.TempDir()
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	dependencyPath := util.JoinPath(tmpEnvPath, testFixtureOutDir, "dependency")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --log-level trace --working-dir %s --out-dir %s", dependencyPath, tmpDir))

	// run plan with output directory
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all plan --non-interactive --log-level trace --working-dir %s --json-out-dir %s --out-dir %s", testPath, tmpDir, tmpDir))
	require.NoError(t, err)

	// verify that was generated json files with plan data
	list, err := findFilesWithExtension(tmpDir, ".json")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	for _, file := range list {
		assert.Equal(t, "tfplan.json", filepath.Base(file))
		// verify that file is not empty
		content, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.NotEmpty(t, content)
	}

	// verify that was generated binary plan files
	list, err = findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}
}

func TestTerragruntRunAllPlanAndShow(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := t.TempDir()
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	dependencyPath := util.JoinPath(tmpEnvPath, testFixtureOutDir, "dependency")
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --log-level trace --working-dir %s --out-dir %s", dependencyPath, tmpDir))

	// run plan and apply
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all plan --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all apply --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	// run new plan and show
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all plan --non-interactive --log-level trace --working-dir %s --out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all show --non-interactive --log-level trace --tf-forward-stdout --working-dir %s --out-dir %s -no-color", testPath, tmpDir))
	require.NoError(t, err)

	// Verify that output contains the plan and not plain the actual state output
	assert.Contains(t, stdout, "No changes. Your infrastructure matches the configuration.")
}

func TestLogFormatJSONOutput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNotExistingSource)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureNotExistingSource)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply --log-format=json --non-interactive --working-dir "+testPath)
	require.Error(t, err)

	// for windows OS
	output := bytes.ReplaceAll([]byte(stderr), []byte("\r\n"), []byte("\n"))

	multipeJSONs := bytes.Split(output, []byte("\n"))

	var msgs = make([]string, 0, len(multipeJSONs))

	for _, jsonBytes := range multipeJSONs {
		if len(jsonBytes) == 0 {
			continue
		}

		var output map[string]any

		err = json.Unmarshal(jsonBytes, &output)
		require.NoError(t, err)

		msg, ok := output["msg"].(string)
		assert.True(t, ok)

		msgs = append(msgs, msg)
	}

	assert.Contains(t, strings.Join(msgs, ""), "Downloading Terraform configurations from git::https://github.com/gruntwork-io/terragrunt.git?ref=v0.83.2")
}

func TestTerragruntOutputFromDependencyLogsJson(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		arg string
	}{
		{"--json"},
		{"--json --log-format json"},
		{"--tf-forward-stdout"},
		{"--json --log-format json --tf-forward-stdout"},
	}
	for _, tc := range testCases {
		t.Run("terragrunt output with "+tc.arg, func(t *testing.T) {
			t.Parallel()
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyOutput)
			rootTerragruntPath := util.JoinPath(tmpEnvPath, testFixtureDependencyOutput)
			// apply dependency first
			dependencyTerragruntConfigPath := util.JoinPath(rootTerragruntPath, "dependency")
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --working-dir %s ", dependencyTerragruntConfigPath))
			require.NoError(t, err)

			appTerragruntConfigPath := util.JoinPath(rootTerragruntPath, "app")
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt plan --non-interactive --working-dir %s %s", appTerragruntConfigPath, tc.arg))
			require.NoError(t, err)

			output := fmt.Sprintf("%s %s", stderr, stdout)
			assert.NotContains(t, output, "invalid character")
		})
	}
}

func TestTerragruntJsonPlanJsonOutput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		tgArgs string
		tfArgs string
	}{
		{"", "--json"},
		{"--log-format json", "--json"},
		{"--tf-forward-stdout", ""},
		{"--log-format json --tf-forward-stdout", "--json"},
	}
	for _, tc := range testCases {
		t.Run("terragrunt with "+tc.tgArgs+" -- plan "+tc.tfArgs, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			_, _, _, err := testRunAllPlan(t, tc.tgArgs+" --json-out-dir "+tmpDir, tc.tfArgs)
			require.NoError(t, err)
			list, err := findFilesWithExtension(tmpDir, ".json")
			require.NoError(t, err)
			assert.Len(t, list, 2)

			for _, file := range list {
				assert.Equal(t, "tfplan.json", filepath.Base(file))
				// verify that file is not empty
				content, err := os.ReadFile(file)
				require.NoError(t, err)
				assert.NotEmpty(t, content)
				// check that produced json is valid and can be unmarshalled
				var plan map[string]any

				err = json.Unmarshal(content, &plan)
				require.NoError(t, err)
				// check that plan is not empty
				assert.NotEmpty(t, plan)
			}
		})
	}
}

func TestErrorMessageIncludeInOutput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureErrorPrint)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureErrorPrint)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply  --non-interactive --working-dir "+testPath+" --tf-path "+testPath+"/custom-tf-script.sh --log-level trace")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "Custom error from script")
}

func TestTerragruntTerraformOutputJson(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInitError)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureInitError)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply --no-color --log-format=json --non-interactive --working-dir "+testPath)
	require.Error(t, err)

	// Sometimes, this is the error returned by AWS.
	if !strings.Contains(stderr, "Error: Failed to get existing workspaces: operation error S3: ListObjectsV2, https response error StatusCode: 301") {
		assert.Regexp(t, `"msg":".*`+regexp.QuoteMeta("Initializing the backend..."), stderr)
	}

	// check if output can be extracted in json
	jsonStrings := strings.SplitSeq(stderr, "\n")
	for jsonString := range jsonStrings {
		if len(jsonString) == 0 {
			continue
		}

		var output map[string]any

		err = json.Unmarshal([]byte(jsonString), &output)
		require.NoErrorf(t, err, "Failed to parse json %s", jsonString)
		assert.NotNil(t, output["level"])
		assert.NotNil(t, output["time"])
	}
}

func TestLogStreaming(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogStreaming)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureLogStreaming)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+testPath+" apply")
	require.NoError(t, err)

	for _, unit := range []string{"unit1", "unit2"} {
		// Find the timestamps for the first and second log entries for this unit
		firstTimestamp := time.Time{}
		secondTimestamp := time.Time{}

		for line := range strings.SplitSeq(stdout, "\n") {
			if strings.Contains(line, unit) {
				if !strings.Contains(line, "(local-exec): sleeping...") && !strings.Contains(line, "(local-exec): done sleeping") {
					continue
				}

				dateTimestampStr := strings.Split(line, " ")[0]
				// The dateTimestampStr looks like this:
				// time=2025-01-09EST15:47:04-05:00
				//
				// We just need the timestamp
				timestampStr := dateTimestampStr[18:26]

				timestamp, err := time.Parse("15:04:05.999", timestampStr)
				require.NoError(t, err)

				if firstTimestamp.IsZero() {
					assert.Contains(t, line, "(local-exec): sleeping...")

					firstTimestamp = timestamp
				} else {
					assert.Contains(t, line, "(local-exec): done sleeping")

					secondTimestamp = timestamp

					break
				}
			}
		}

		// Confirm that the timestamps are at least 1 second apart
		require.GreaterOrEqualf(t, secondTimestamp.Sub(firstTimestamp), 1*time.Second, "Second log entry for unit %s is not at least 1 second after the first log entry", unit)
	}
}

func TestLogFormatBare(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureEmptyState)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureEmptyState)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt init --log-format=bare --no-color --non-interactive --working-dir "+testPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Initializing the backend...")
	assert.NotContains(t, stdout, "STDO[0000] Initializing the backend...")
}

func TestTF110EphemeralVars(t *testing.T) {
	t.Parallel()

	if !helpers.IsTerraform110OrHigher(t) {
		t.Skip("This test requires Terraform 1.10 or higher")

		return
	}

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureEphemeralInputs)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureEphemeralInputs)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive --working-dir "+testPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Plan: 1 to add, 0 to change, 0 to destroy")

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply --auto-approve --non-interactive --working-dir "+testPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed")
}

//nolint:paralleltest
func TestTfPath(t *testing.T) {
	// This test can't be parallelized because it explicitly unsets the TG_TF_PATH environment variable.
	// t.Parallel()

	// Test that the terragrunt run version command correctly identifies and uses
	// the terraform_binary path configuration if present
	helpers.CleanupTerraformFolder(t, testFixtureTfPathBasic)
	rootPath := helpers.CopyEnvironment(t, testFixtureTfPathBasic)
	workingDir := util.JoinPath(rootPath, testFixtureTfPathBasic)
	workingDir, err := filepath.EvalSymlinks(workingDir)
	require.NoError(t, err)

	// If TG_TF_PATH is not set, we'll use the default tofu binary,
	// we'll explicitly set the value so that the test can pass.
	if tfPath := os.Getenv("TG_TF_PATH"); tfPath != "" {
		// Unset after using t.Setenv so that it'll be reset after the test.
		t.Setenv("TG_TF_PATH", "")
		os.Unsetenv("TG_TF_PATH")
	}

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run version --working-dir "+workingDir)
	require.NoError(t, err)

	assert.Contains(t, stderr, "TF script used!")
}

func TestTfPathOverridesConfig(t *testing.T) {
	t.Parallel()
	// Test that the terragrunt run version command correctly identifies and uses
	// the terraform_binary path configuration if present
	helpers.CleanupTerraformFolder(t, testFixtureTfPathBasic)
	rootPath := helpers.CopyEnvironment(t, testFixtureTfPathBasic)
	workingDir := util.JoinPath(rootPath, testFixtureTfPathBasic)
	workingDir, err := filepath.EvalSymlinks(workingDir)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run version --tf-path ./other-tf.sh --working-dir "+workingDir)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Other TF script used!")
}

func TestTfPathOverridesConfigWithTofuTerraform(t *testing.T) {
	t.Parallel()

	// This test requires that both tofu and terraform are installed.
	if !helpers.IsTerraformInstalled() || !helpers.IsOpenTofuInstalled() {
		t.Skip("This test requires that both OpenTofu and Terraform are installed")

		return
	}

	helpers.CleanupTerraformFolder(t, testFixtureTfPathTofuTerraform)
	rootPath := helpers.CopyEnvironment(t, testFixtureTfPathTofuTerraform)
	workingDir := util.JoinPath(rootPath, testFixtureTfPathTofuTerraform)
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

func TestMixedStackConfigIgnored(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMixedConfig)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureMixedConfig)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+testPath+" -- apply")
	require.NoError(t, err)
	require.NotContains(t, stderr, "Error: Unsupported block type")
	require.NotContains(t, stderr, "Blocks of type \"unit\" are not expected here")
}

// Test that default command forwarding is disabled and users are guided to use `run --`.
func TestNoDefaultForwardingUnknownCommand(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt workspace list --non-interactive --working-dir "+rootPath)
	require.Error(t, err, "expected error when invoking unknown top-level command without 'run'")
}

func TestDiscoveryDoesntResolveOutputs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	depDir := filepath.Join(tmpDir, "dep")
	err := os.MkdirAll(depDir, 0755)
	require.NoError(t, err)

	mainDir := filepath.Join(tmpDir, "main")
	err = os.MkdirAll(mainDir, 0755)
	require.NoError(t, err)

	depConfig := `
terraform {
  source = "."
}
`
	err = os.WriteFile(filepath.Join(depDir, "terragrunt.hcl"), []byte(depConfig), 0644)
	require.NoError(t, err)

	depTerraform := `
output "value" {
  value = "hello from dependency"
}
`
	err = os.WriteFile(filepath.Join(depDir, "main.tf"), []byte(depTerraform), 0644)
	require.NoError(t, err)

	mainConfig := `
terraform {
  source = "."
}

dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    value = "mock value"
  }
}

inputs = {
  dep_value = dependency.dep.outputs.value
}
`
	err = os.WriteFile(filepath.Join(mainDir, "terragrunt.hcl"), []byte(mainConfig), 0644)
	require.NoError(t, err)

	mainTerraform := `
variable "dep_value" {
  type = string
}

output "result" {
  value = var.dep_value
}
`
	err = os.WriteFile(filepath.Join(mainDir, "main.tf"), []byte(mainTerraform), 0644)
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+depDir)
	require.NoError(t, err)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --non-interactive --working-dir "+depDir)
	require.NoError(t, err)
	assert.Contains(t, stdout, "hello from dependency")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all apply --non-interactive --working-dir "+tmpDir)
	require.NoError(t, err)

	assert.NotEmpty(t, stdout)
	assert.NotEmpty(t, stderr)

	assert.NotContains(t, stderr, "that has no outputs, but mock outputs provided and returning those in dependency output")

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --non-interactive --working-dir "+mainDir)
	require.NoError(t, err)

	assert.Contains(t, stdout, "hello from dependency")
}

func TestExternalDependenciesAreResolved(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	depDir := filepath.Join(tmpDir, "dep")
	err := os.MkdirAll(depDir, 0755)
	require.NoError(t, err)

	mainDir := filepath.Join(tmpDir, "main")
	err = os.MkdirAll(mainDir, 0755)
	require.NoError(t, err)

	depConfig := `
terraform {
  source = "."
}
`
	err = os.WriteFile(filepath.Join(depDir, "terragrunt.hcl"), []byte(depConfig), 0644)
	require.NoError(t, err)

	depTerraform := `
output "value" {
  value = "hello from dependency"
}
`
	err = os.WriteFile(filepath.Join(depDir, "main.tf"), []byte(depTerraform), 0644)
	require.NoError(t, err)

	mainConfig := `
terraform {
  source = "."
}

dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    value = "mock value"
  }
}

inputs = {
  dep_value = dependency.dep.outputs.value
}
`
	err = os.WriteFile(filepath.Join(mainDir, "terragrunt.hcl"), []byte(mainConfig), 0644)
	require.NoError(t, err)

	mainTerraform := `
variable "dep_value" {
  type = string
}

output "result" {
  value = var.dep_value
}
`
	err = os.WriteFile(filepath.Join(mainDir, "main.tf"), []byte(mainTerraform), 0644)
	require.NoError(t, err)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all plan --non-interactive --queue-exclude-external --working-dir "+mainDir,
	)
	require.NoError(t, err)

	assert.NotEmpty(t, stdout)
	assert.NotEmpty(t, stderr)

	assert.Contains(
		t,
		stderr,
		"that has no outputs, but mock outputs provided and returning those in dependency output",
	)
	assert.NotContains(
		t,
		stderr,
		`There is no variable named "dependency".`,
	)
}

func TestRunAllDetectsHiddenDirectories(t *testing.T) {
	t.Parallel()
	rootPath := helpers.CopyEnvironment(t, hiddenRunAllFixturePath, ".cloud/**")
	modulePath := util.JoinPath(rootPath, hiddenRunAllFixturePath)
	helpers.CleanupTerraformFolder(t, modulePath)

	// Expect Terragrunt to discover modules under .cloud directory
	command := "terragrunt run --all plan --non-interactive --log-level trace --working-dir " + modulePath
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, command)

	require.NoError(t, err)
	assert.Contains(t, stdout, "hidden1")
	assert.Contains(t, stdout, "hidden2")
}
