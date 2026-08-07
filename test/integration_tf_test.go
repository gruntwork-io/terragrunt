//go:build tf

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

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFDetailedExitCodeError(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "error")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath)

	exitCode := tf.NewDetailedExitCodeMap()

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	_, stderr, err := helpers.RunTerragruntCommandWithOutputWithContext(
		t,
		ctx,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode",
	)
	require.Error(t, err)
	assert.Contains(t, stderr, "not-existing-file.txt: no such file or directory")
	assert.Equal(t, 1, exitCode.GetFinalDetailedExitCode())
}

// TestTFRunAllReturnsErrorOnFailure verifies that `terragrunt run --all` returns
// a non-zero exit code when one of the units fails. This is a regression test
// for https://github.com/gruntwork-io/terragrunt/issues/5379
func TestTFRunAllReturnsErrorOnFailure(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "error")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --log-level trace --non-interactive --working-dir "+rootPath+" -- plan",
	)
	require.Error(t, err)
	assert.Contains(t, stderr, "not-existing-file.txt: no such file or directory")
}

func TestTFDetailedExitCodeChangesPresentAll(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "changes")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath)

	exitCode := tf.NewDetailedExitCodeMap()

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	_, _, err := helpers.RunTerragruntCommandWithOutputWithContext(
		t,
		ctx,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode",
	)
	require.NoError(t, err)
	assert.Equal(t, 2, exitCode.GetFinalDetailedExitCode())
}

func TestTFDetailedExitCodeChangesUnit(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "changes")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath)
	ctx := t.Context()

	_, _, err := helpers.RunTerragruntCommandWithOutputWithContext(
		t,
		ctx,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply",
	)
	require.NoError(t, err)

	// delete example.txt from cache directory to have changes in one unit
	// The file is created in the cache directory since source is always copied there
	app1CacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(rootPath, "app1"))
	require.NotEmpty(t, app1CacheDir, "Should find cache working directory for app1")
	err = os.Remove(filepath.Join(app1CacheDir, "example.txt"))
	require.NoError(t, err)

	// check that the exit code is 2 when there are changes in one unit
	exitCode := tf.NewDetailedExitCodeMap()

	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	_, _, err = helpers.RunTerragruntCommandWithOutputWithContext(
		t,
		ctx,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode",
	)
	require.NoError(t, err)
	assert.Equal(t, 2, exitCode.GetFinalDetailedExitCode())
}

func TestTFDetailedExitCodeFailOnFirstRun(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "fail-on-first-run")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)

	exitCode := tf.NewDetailedExitCodeMap()

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	_, _, err := helpers.RunTerragruntCommandWithOutputWithContext(
		t,
		ctx,
		"terragrunt run --all --non-interactive --working-dir "+filepath.Join(
			tmpEnvPath,
			testFixturePath,
		)+" -- plan -detailed-exitcode",
	)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode.GetFinalDetailedExitCode())
}

func TestTFDetailedExitCodeFailOnFirstRunWithStatus(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "fail-on-first-run-with-status")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)

	exitCode := tf.NewDetailedExitCodeMap()

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	_, _, err := helpers.RunTerragruntCommandWithOutputWithContext(
		t,
		ctx,
		"terragrunt run --working-dir "+filepath.Join(
			tmpEnvPath,
			testFixturePath,
		)+" -- plan -detailed-exitcode",
	)
	require.NoError(t, err)
	assert.Equal(t, 2, exitCode.GetFinalDetailedExitCode())
}

func TestTFDetailedExitCodeFailOnFirstRunAllWithStatus(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "fail-on-first-run-with-status")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)

	exitCode := tf.NewDetailedExitCodeMap()

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	_, _, err := helpers.RunTerragruntCommandWithOutputWithContext(
		t,
		ctx,
		"terragrunt run --working-dir "+filepath.Join(
			tmpEnvPath,
			testFixturePath,
		)+" --all -- plan -detailed-exitcode",
	)
	require.NoError(t, err)
	assert.Equal(t, 2, exitCode.GetFinalDetailedExitCode())
}

func TestTFDetailedExitCodeChangesPresentOne(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "changes")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath)

	exitCode := tf.NewDetailedExitCodeMap()

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+filepath.Join(
			rootPath,
			"app1",
		),
	)
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutputWithContext(
		t,
		ctx,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode",
	)
	require.NoError(t, err)
	assert.Equal(t, 2, exitCode.GetFinalDetailedExitCode())
}

func TestTFDetailedExitCodeNoChanges(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "changes")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath)

	exitCode := tf.NewDetailedExitCodeMap()

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutputWithContext(
		t,
		ctx,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode",
	)
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode.GetFinalDetailedExitCode())
}

func TestTFRunAllDetailedExitCode_RetryableAfterDrift(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "runall-retry-after-drift")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath)

	// Pre-apply the drift unit so it has a file, then delete it to ensure drift exists
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+
			filepath.Join(rootPath, "app_drift"),
	)
	require.NoError(t, err)
	// Delete file from cache directory since that's where it was created
	appDriftCacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(rootPath, "app_drift"))
	require.NotEmpty(t, appDriftCacheDir, "Should find cache working directory for app_drift")
	err = os.Remove(filepath.Join(appDriftCacheDir, "example.txt"))
	require.NoError(t, err)

	exitCode := tf.NewDetailedExitCodeMap()

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	_, _, err = helpers.RunTerragruntCommandWithOutputWithContext(
		t, ctx,
		"terragrunt run --all --non-interactive --working-dir "+
			rootPath+
			" -- plan -detailed-exitcode",
	)
	require.NoError(t, err)
	assert.Equal(t, 2, exitCode.GetFinalDetailedExitCode())
}

// TestTFDetailedExitCodeChangesPresentAllWithSource verifies that run --all correctly
// propagates the detailed exit code when units use terraform { source = "." }.
// This is a regression test for https://github.com/gruntwork-io/terragrunt/issues/5586
func TestTFDetailedExitCodeChangesPresentAllWithSource(t *testing.T) {
	t.Parallel()

	testFixturePath := filepath.Join(testFixtureDetailedExitCode, "changes-with-source")

	helpers.CleanupTerraformFolder(t, testFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePath)
	rootPath := filepath.Join(tmpEnvPath, testFixturePath)

	exitCode := tf.NewDetailedExitCodeMap()

	ctx := t.Context()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	_, _, err := helpers.RunTerragruntCommandWithOutputWithContext(
		t,
		ctx,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- plan -detailed-exitcode",
	)
	require.NoError(t, err)
	assert.Equal(t, 2, exitCode.GetFinalDetailedExitCode())
}

func TestTFLogCustomFormatOutput(t *testing.T) {
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
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(
						" plain-text STDOUT dep "+wrappedBinary(
							t.Context(),
						)+" init -no-color -input=false: Initializing the backend...",
					),
				),
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(
						" plain-text STDOUT app "+wrappedBinary(
							t.Context(),
						)+" init -no-color -input=false: Initializing the backend...",
					),
				),
			},
			expectedStdErrRegs: []*regexp.Regexp{
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(" plain-text DEBUG  Terragrunt Version:"),
				),
			},
		},
		{
			logCustomFormat: "%interval%(content=' plain-text ')%level(case=upper,width=6) %prefix(path=short-relative,suffix=' ')%tf-path(suffix=' ')%tf-command(suffix=': ')%msg(path=relative)",
			expectedStdOutRegs: []*regexp.Regexp{
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(
						" plain-text STDOUT dep "+wrappedBinary(
							t.Context(),
						)+" init: Initializing the backend...",
					),
				),
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(
						" plain-text STDOUT app "+wrappedBinary(
							t.Context(),
						)+" init: Initializing the backend...",
					),
				),
			},
			expectedStdErrRegs: []*regexp.Regexp{
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(" plain-text DEBUG  Terragrunt Version:"),
				),
			},
		},
		{
			logCustomFormat: "%interval%(content=' plain-text ')%level(case=upper,width=6) %prefix(path=short-relative,suffix=' ')%tf-path(suffix=' ')%tf-command()-args %msg(path=relative)",
			expectedStdOutRegs: []*regexp.Regexp{
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(
						" plain-text STDOUT dep "+wrappedBinary(
							t.Context(),
						)+" init-args Initializing the backend...",
					),
				),
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(
						" plain-text STDOUT app "+wrappedBinary(
							t.Context(),
						)+" init-args Initializing the backend...",
					),
				),
			},
			expectedStdErrRegs: []*regexp.Regexp{
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(" plain-text DEBUG  -args Terragrunt Version:"),
				),
			},
		},
		{
			logCustomFormat: "%interval%(content=' plain-text ')%level(case=upper,width=6) %prefix(path=short-relative,suffix=' ')%tf-path(suffix=' ')%tf-command()-args % aaa %msg(path=relative) %%bbb % ccc",
			expectedStdOutRegs: []*regexp.Regexp{
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(
						" plain-text STDOUT dep "+wrappedBinary(
							t.Context(),
						)+" init-args % aaa Initializing the backend... %bbb % ccc",
					),
				),
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(
						" plain-text STDOUT app "+wrappedBinary(
							t.Context(),
						)+" init-args % aaa Initializing the backend... %bbb % ccc",
					),
				),
			},
			expectedStdErrRegs: []*regexp.Regexp{
				regexp.MustCompile(
					`\d{4}` + regexp.QuoteMeta(
						" plain-text DEBUG  -args % aaa Terragrunt Version:",
					),
				),
			},
		},
		{
			logCustomFormat: "%time(color=green) %level %wrong",
			expectedErr: fmt.Errorf(
				`invalid value "%%time(color=green) %%level %%wrong" for flag -log-custom-format: invalid placeholder name "wrong", available names: %s`,
				strings.Join(placeholders.NewPlaceholderRegister().Names(), ","),
			),
		},
		{
			logCustomFormat: "%time(colorr=green) %level",
			expectedErr: fmt.Errorf(
				`invalid value "%%time(colorr=green) %%level" for flag -log-custom-format: placeholder "time", invalid option name "colorr", available names: %s`,
				strings.Join(placeholders.Time().Options().Names(), ","),
			),
		},
		{
			logCustomFormat: "%time(color=green) %level(format=tinyy)",
			expectedErr: errors.New(
				`invalid value "%time(color=green) %level(format=tinyy)" for flag -log-custom-format: placeholder "level", option "format", invalid value "tinyy", available values: full,short,tiny`,
			),
		},
		{
			logCustomFormat: "%time(=green) %level(format=tiny)",
			expectedErr: errors.New(
				`invalid value "%time(=green) %level(format=tiny)" for flag -log-custom-format: placeholder "time", empty option name "=green) %level(format=tiny)"`,
			),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
			rootPath := filepath.Join(tmpEnvPath, testFixtureLogFormatter)

			rootPath, err := filepath.EvalSymlinks(rootPath)
			require.NoError(t, err)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				fmt.Sprintf(
					"terragrunt run --all --log-level debug --non-interactive --no-color --log-custom-format=%q --working-dir %s -- init -no-color",
					tc.logCustomFormat,
					rootPath,
				),
			)

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

func TestTFBufferModuleOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureBufferModuleOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureBufferModuleOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureBufferModuleOutput)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --log-disable --working-dir "+rootPath+" -- plan -out planfile",
	)
	require.NoError(t, err)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --log-disable --working-dir "+rootPath+" -- show -json planfile",
	)
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

func TestTFDisableLogging(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLogFormatter)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all init --log-disable --non-interactive -no-color --no-color --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Initializing provider plugins...")
	assert.Empty(t, stderr)
}

func TestTFLogWithAbsPath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLogFormatter)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all init --log-show-abs-paths --log-level debug --non-interactive -no-color --no-color --log-format=pretty --working-dir "+rootPath,
	)
	require.NoError(t, err)

	for _, prefixName := range []string{"app", "dep"} {
		prefixName = filepath.Join(rootPath, prefixName)
		assert.Contains(
			t,
			stdout,
			"STDOUT ["+prefixName+"] "+wrappedBinary(
				t.Context(),
			)+": Initializing provider plugins...",
		)
		assert.Contains(
			t,
			stderr,
			"DEBUG  ["+prefixName+"] Reading Terragrunt config file at "+prefixName+"/terragrunt.hcl",
		)
	}
}

func TestTFLogWithRelPath(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogRelPaths)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogRelPaths)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLogRelPaths)

	testCases := []struct {
		assertFn   func(t *testing.T, stdout, stderr string)
		workingDir string
	}{
		{
			workingDir: "duplicate-dir-names/workspace/one/two/aaa", // dir `workspace` duplicated twice in path
			assertFn: func(t *testing.T, _, stderr string) {
				t.Helper()

				assert.Contains(
					t,
					stderr,
					"Downloading Terraform configurations from .. into ./bbb/ccc/workspace/.terragrunt-cache",
				)
				assert.Contains(t, stderr, "[bbb/ccc/workspace]")
				assert.Contains(t, stderr, "[bbb/ccc/module-b]")
			},
		},
	}

	for i, tc := range testCases {
		workingDir := filepath.Join(rootPath, tc.workingDir)

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt run --all init --non-interactive --no-color --log-format=pretty --working-dir "+workingDir,
			)
			require.NoError(t, err)

			tc.assertFn(t, stdout, stderr)
		})
	}
}

func TestTFLogFormatPrettyOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLogFormatter)

	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all init --log-level debug --non-interactive --no-color --log-format=pretty  --working-dir "+rootPath,
	)
	require.NoError(t, err)

	for _, prefixName := range []string{"app", "dep"} {
		assert.Contains(
			t,
			stdout,
			"STDOUT ["+prefixName+"] "+wrappedBinary(
				t.Context(),
			)+": Initializing provider plugins...",
		)
		assert.Contains(
			t,
			stderr,
			"DEBUG  ["+prefixName+"] Reading Terragrunt config file at ./"+prefixName+"/terragrunt.hcl",
		)
	}

	assert.NotEmpty(t, stdout)
	assert.Contains(t, stderr, "DEBUG  Terragrunt Version:")
}

func TestTFLogStdoutLevel(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogStdoutLevel)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogStdoutLevel)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLogStdoutLevel)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive -no-color --no-color --log-format=pretty  --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stdout, "STDOUT "+wrappedBinary(t.Context())+": Changes to Outputs")

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt destroy -auto-approve --non-interactive -no-color --no-color --log-format=pretty  --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stdout, "STDOUT "+wrappedBinary(t.Context())+": Changes to Outputs")
}

func TestTFLogFormatKeyValueOutput(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{"--log-format=key-value"} {
		t.Run("tc-flag-"+flag, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
			rootPath := filepath.Join(tmpEnvPath, testFixtureLogFormatter)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt run --all --log-level debug --non-interactive "+flag+" --working-dir "+rootPath+" -- init -no-color",
			)
			require.NoError(t, err)

			for _, prefixName := range []string{"app", "dep"} {
				assert.Contains(
					t,
					stdout,
					"level=stdout prefix="+prefixName+" tf-path="+wrappedBinary(
						t.Context(),
					)+" msg=Initializing provider plugins...\n",
				)
				assert.Contains(
					t,
					stderr,
					"level=debug prefix="+prefixName+" msg=Reading Terragrunt config file at ./"+prefixName+"/terragrunt.hcl\n",
				)
			}
		})
	}
}

func TestTFLogRawModuleOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLogFormatter)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive  --tf-forward-stdout --working-dir "+rootPath+" -- init -no-color",
	)
	require.NoError(t, err)

	stdoutInline := strings.ReplaceAll(stdout, "\n", "")
	assert.Contains(t, stdoutInline, "Initializing the backend...Initializing provider plugins...")
	assert.NotRegexp(t, `(?i)(`+strings.Join(log.AllLevels.Names(), "|")+`)+`, stdoutInline)
}

func TestTFTerragruntExcludesFile(t *testing.T) {
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

			tmpEnvPath := helpers.CopyEnvironment(
				t,
				testFixtureExcludesFile,
				".terragrunt-excludes",
			)
			rootPath := filepath.Join(tmpEnvPath, testFixtureExcludesFile)

			helpers.RunTerragrunt(
				t,
				fmt.Sprintf(
					"terragrunt run apply --all --non-interactive --working-dir %s %s -- -auto-approve",
					rootPath,
					tc.flags,
				),
			)

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(
				t,
				fmt.Sprintf(
					"terragrunt run output --all --non-interactive --working-dir %s %s",
					rootPath,
					tc.flags,
				),
			)
			require.NoError(t, err)

			actualOutput := strings.Split(strings.TrimSpace(stdout), "\n")
			assert.ElementsMatch(t, tc.expectedOutput, actualOutput)
		})
	}
}

func TestTFTerragruntProviderCacheMultiplePlatforms(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureProviderCacheMultiplePlatforms)

	// OpenTofu and Terraform accept the platform value attached to the flag or as the argument that follows it.
	testCases := []struct {
		name               string
		flagValueSeparator string
	}{
		{
			name:               "attached value",
			flagValueSeparator: "=",
		},
		{
			name:               "separate value",
			flagValueSeparator: " ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureProviderCacheMultiplePlatforms)
			rootPath := filepath.Join(tmpEnvPath, testFixtureProviderCacheMultiplePlatforms)

			providerCacheDir := helpers.TmpDirWOSymlinks(t)

			var (
				platforms     = []string{"linux_amd64", "darwin_arm64"}
				platformsArgs = make([]string, 0, len(platforms))
			)

			for _, platform := range platforms {
				platformsArgs = append(platformsArgs, "-platform"+tc.flagValueSeparator+platform)
			}

			helpers.RunTerragrunt(
				t,
				fmt.Sprintf(
					"terragrunt run --all --no-auto-init --provider-cache --provider-cache-dir %s --non-interactive --working-dir %s",
					providerCacheDir,
					rootPath,
				)+" -- providers lock "+strings.Join(
					platformsArgs,
					" ",
				),
			)

			providers := []string{
				"hashicorp/null/3.2.3",
				"hashicorp/local/2.5.2",
			}

			registryName := "registry.opentofu.org"
			if isTerraform(t.Context()) {
				registryName = "registry.terraform.io"
			}

			for _, appName := range []string{"app1", "app2", "app3"} {
				appPath := filepath.Join(rootPath, appName)
				assert.DirExists(t, appPath)

				lockfilePath := filepath.Join(appPath, ".terraform.lock.hcl")
				lockfileContent, err := os.ReadFile(lockfilePath)
				require.NoError(t, err)

				lockfile, diags := hclwrite.ParseConfig(
					lockfileContent,
					lockfilePath,
					hcl.Pos{Line: 1, Column: 1},
				)
				assert.False(t, diags.HasErrors())
				assert.NotNil(t, lockfile)

				for _, provider := range providers {
					provider := path.Join(registryName, provider)

					providerBlock := lockfile.Body().
						FirstMatchingBlock("provider", []string{filepath.Dir(provider)})
					assert.NotNil(t, providerBlock)

					providerPath := filepath.Join(providerCacheDir, provider)
					assert.True(t, vfs.Exists(vfs.NewOSFS(), providerPath))

					for _, platform := range platforms {
						platformPath := filepath.Join(providerPath, platform)
						assert.True(t, vfs.Exists(vfs.NewOSFS(), platformPath))
					}
				}
			}
		})
	}
}

func TestTFTerragruntInitOnce(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInitOnce)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInitOnce)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+rootPath,
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Initializing modules")

	// update the config creation time without changing content
	cfgPath := filepath.Join(rootPath, "terragrunt.hcl")
	bytes, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	err = os.WriteFile(cfgPath, bytes, 0o644)
	require.NoError(t, err)

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(
		t, "terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+rootPath,
	)
	require.NoError(t, err)
	assert.NotContains(t, stdout, "Initializing modules", "init command executed more than once")
}

func TestTFTerragruntWorksWithSingleJSONConfig(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureConfigSingleJSONPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureConfigSingleJSONPath)

	rootTerragruntConfigPath := filepath.Join(tmpEnvPath, testFixtureConfigSingleJSONPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt plan --non-interactive --working-dir "+rootTerragruntConfigPath,
	)
}

func TestTFTerragruntWorksWithNonDefaultConfigNamesAndRunAllCommand(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureConfigWithNonDefaultNames)
	tmpEnvPath = path.Join(tmpEnvPath, testFixtureConfigWithNonDefaultNames)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --log-level debug --config main.hcl --non-interactive --working-dir "+tmpEnvPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, "run_cmd output: [parent_hcl_file]")
	assert.Contains(t, stderr, "run_cmd output: [dependency_hcl]")
	assert.Contains(t, stderr, "run_cmd output: [common_hcl]")
}

func TestTFTerragruntWorksWithNonDefaultConfigNames(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureConfigWithNonDefaultNames)
	tmpEnvPath = path.Join(tmpEnvPath, testFixtureConfigWithNonDefaultNames)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply --config main.hcl --non-interactive --working-dir "+
			filepath.Join(tmpEnvPath, "app"),
	)
	require.NoError(t, err)

	assert.Equal(t, 1, strings.Count(stdout, "parent_hcl_file"))
	assert.Equal(t, 1, strings.Count(stdout, "dependency_hcl"))
	assert.Equal(t, 1, strings.Count(stdout, "common_hcl"))
}

func TestTFTerragruntReportsTerraformErrorsWithPlanAll(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFailedTerraform)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFailedTerraform)

	rootTerragruntConfigPath := filepath.Join(tmpEnvPath, "fixtures/failure")

	cmd := "terragrunt run --all plan --non-interactive --working-dir " + rootTerragruntConfigPath

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	// Call helpers.RunTerragruntCommand directly because this command contains failures (which causes helpers.RunTerragruntRedirectOutput to abort) but we don't care.
	err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)
	require.Error(t, err)

	output := stdout.String()
	errOutput := stderr.String()
	fmt.Printf("STDERR is %s.\n STDOUT is %s", errOutput, output)

	assert.Contains(t, errOutput, "missingvar1")
	assert.Contains(t, errOutput, "missingvar2")
}

// Check that Terragrunt does not pollute stdout with anything
func TestTFTerragruntStdOut(t *testing.T) {
	t.Parallel()

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+testFixtureStdout,
	)
	helpers.RunTerragruntRedirectOutput(
		t,
		"terragrunt output foo --non-interactive --working-dir "+testFixtureStdout,
		&stdout,
		&stderr,
	)

	output := stdout.String()
	assert.Equal(t, "\"foo\"\n", output)
}

func TestTFTerragruntStackCommandsWithPlanFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := filepath.EvalSymlinks(helpers.CopyEnvironment(t, testFixtureDisjoint))
	require.NoError(t, err)

	disjointEnvironmentPath := filepath.Join(tmpEnvPath, testFixtureDisjoint)

	helpers.CleanupTerraformFolder(t, disjointEnvironmentPath)
	helpers.RunTerragrunt(
		t,
		"terragrunt run --all  --log-level info --non-interactive --working-dir "+disjointEnvironmentPath+" -- plan -out=plan.tfplan",
	)
	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --log-level info --non-interactive --working-dir "+disjointEnvironmentPath+" -- apply plan.tfplan",
	)
}

func TestTFInvalidSource(t *testing.T) {
	t.Parallel()

	mirror := helpers.NewGitServer(t)
	tmpEnvPath := mirror.RenderFixture(testFixtureNotExistingSource)
	generateTestCase := filepath.Join(tmpEnvPath, testFixtureNotExistingSource)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt init --working-dir "+generateTestCase,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	var workingDirNotFoundErr run.WorkingDirNotFound

	ok := errors.As(err, &workingDirNotFoundErr)
	assert.True(t, ok)
}

func TestTFPlanfileOrder(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixturePlanfileOrder)
	modulePath := filepath.Join(rootPath, testFixturePlanfileOrder)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --working-dir "+modulePath,
		os.Stdout,
		os.Stderr,
	)
	require.NoError(t, err)

	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --working-dir "+modulePath,
		os.Stdout,
		os.Stderr,
	)
	require.NoError(t, err)
}

// This tests terragrunt properly passes through terraform commands and any number of specified args
func TestTFTerraformCommandCliArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr error
		expected    string
		command     []string
	}{
		{
			command:  []string{"version"},
			expected: wrappedBinary(t.Context()) + " version",
		},
		{
			command:  []string{"--", "version"},
			expected: wrappedBinary(t.Context()) + " version",
		},
		{
			command:  []string{"--", "version", "foo"},
			expected: wrappedBinary(t.Context()) + " version",
		},
		{
			command:  []string{"--", "version", "foo", "bar", "baz"},
			expected: wrappedBinary(t.Context()) + " version",
		},
		{
			command:  []string{"--", "version", "foo", "bar", "baz", "foobar"},
			expected: wrappedBinary(t.Context()) + " version",
		},
		{
			command:  []string{"--", "graph"},
			expected: "digraph",
		},
	}

	for _, tc := range testCases {
		cmd := fmt.Sprintf(
			"terragrunt run --non-interactive --log-level debug --working-dir %s %s",
			testFixtureExtraArgsPath,
			strings.Join(
				tc.command,
				" ",
			),
		)

		stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
		if tc.expectedErr != nil {
			require.ErrorIs(t, err, tc.expectedErr)
		}

		assert.Contains(t, stdout+stderr, tc.expected)
	}
}

// This tests terragrunt properly passes through terraform commands with sub commands
// and any number of specified args
func TestTFTerraformSubcommandCliArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expected string
		command  []string
	}{
		{
			command:  []string{"force-unlock"},
			expected: wrappedBinary(t.Context()) + " force-unlock",
		},
		{
			command:  []string{"force-unlock", "foo"},
			expected: wrappedBinary(t.Context()) + " force-unlock foo",
		},
		{
			command:  []string{"force-unlock", "foo", "bar", "baz"},
			expected: wrappedBinary(t.Context()) + " force-unlock foo bar baz",
		},
		{
			command:  []string{"force-unlock", "foo", "bar", "baz", "foobar"},
			expected: wrappedBinary(t.Context()) + " force-unlock foo bar baz foobar",
		},
	}

	for _, tc := range testCases {
		cmd := fmt.Sprintf(
			"terragrunt %s --non-interactive --log-level debug --working-dir %s",
			strings.Join(
				tc.command,
				" ",
			),
			testFixtureExtraArgsPath,
		)

		// Call helpers.RunTerragruntCommand directly because this command
		// contains failures (which causes helpers.RunTerragruntRedirectOutput to abort) but we don't care.
		stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
		if err == nil {
			t.Fatalf("Failed to properly fail command: %v.", cmd)
		}

		assert.True(
			t,
			strings.Contains(stderr, tc.expected) || strings.Contains(stdout, tc.expected),
		)
	}
}

func TestTFInputsPassedThroughCorrectly(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInputs)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	validateInputs(t, outputs)
}

func TestTFRunCommand(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInputs)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt run -no-color --non-interactive --working-dir "+rootPath+" -- output -json",
		&stdout,
		&stderr,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	validateInputs(t, outputs)
}

// TestTFInputsWithInterpolationPatterns validates that input variables containing ${...} patterns
// are passed to Terraform without triggering HCL interpolation errors (issue #3368).
func TestTFInputsWithInterpolationPatterns(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInputsInterpolation)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputsInterpolation)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInputsInterpolation)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	// map_with_interpolation.foo should be the literal string "test ${bar} test" (not interpolated)
	mapOutput, ok := outputs["map_with_interpolation"]
	require.True(t, ok, "map_with_interpolation output not found")
	mapValue, ok := mapOutput.Value.(map[string]any)
	require.True(t, ok, "map_with_interpolation value is not a map")
	assert.Equal(t, "test ${bar} test", mapValue["foo"])
	assert.Equal(t, "no interpolation here", mapValue["baz"])
}

func TestTFTerragruntMissingDependenciesFail(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/missing-dependencies")
	generateTestCase := filepath.Join(tmpEnvPath, testFixtureMissingDependence)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt init --working-dir "+generateTestCase,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	var parsedError config.DependencyDirNotFoundError

	require.ErrorAs(t, err, &parsedError)
	require.Len(t, parsedError.Dir, 1)
	assert.Contains(t, parsedError.Dir[0], "hl3-release")
}

func TestTFTerragruntExcludeExternalDependencies(t *testing.T) {
	t.Parallel()

	excludedModule := "module-a"
	includedModule := "module-b"

	modules := []string{
		excludedModule,
		includedModule,
	}

	helpers.CleanupTerraformFolder(t, testFixtureExternalDependence)

	for _, module := range modules {
		helpers.CleanupTerraformFolder(t, filepath.Join(testFixtureExternalDependence, module))
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	rootPath := helpers.CopyEnvironment(t, testFixtureExternalDependence)
	modulePath := filepath.Join(rootPath, testFixtureExternalDependence, includedModule)

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

func TestTFApplySkipTrue(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSkipLegacyRoot)
	rootPath := filepath.Join(tmpEnvPath, testFixtureSkipLegacyRoot, "skip-true")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --log-level info --non-interactive --working-dir %s --var person=Hobbs",
			rootPath,
		),
		&showStdout,
		&showStderr,
	)
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

func TestTFApplySkipFalse(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureSkipLegacyRoot)
	rootPath = filepath.Join(rootPath, testFixtureSkipLegacyRoot, "skip-false")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --tf-forward-stdout --working-dir "+rootPath,
		&showStdout,
		&showStderr,
	)
	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	stderr := showStderr.String()
	stdout := showStdout.String()

	require.NoError(t, err)
	assert.Contains(t, stdout, "hello, Hobbs")
	assert.NotContains(t, stderr, "Early exit in terragrunt unit")
}

func TestTFApplyAllSkipTrue(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureSkip)
	rootPath = filepath.Join(rootPath, testFixtureSkip, "skip-true")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		fmt.Sprintf(
			"terragrunt run --all apply --non-interactive --tf-forward-stdout --working-dir %s --log-level info",
			rootPath,
		),
		&showStdout,
		&showStderr,
	)
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

func TestTFApplyAllSkipFalse(t *testing.T) {
	t.Parallel()

	rootPath := helpers.CopyEnvironment(t, testFixtureSkip)
	rootPath = filepath.Join(rootPath, testFixtureSkip, "skip-false")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt run --all apply --non-interactive --tf-forward-stdout --working-dir "+rootPath,
		&showStdout,
		&showStderr,
	)
	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	stdout := showStdout.String()
	stderr := showStderr.String()

	require.NoError(t, err)
	assert.Contains(t, stdout, "hello, Ernie")
	assert.Contains(t, stdout, "hello, Bert")
	assert.NotContains(t, stderr, "Early exit in terragrunt unit")
}

func TestTFDependencyOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "integration")

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	// verify expected output 42
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	app3Path := filepath.Join(rootPath, "app3")
	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+app3Path,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, 42, int(outputs["z"].Value.(float64)))
}

func TestTFDependencyOutputErrorBeforeApply(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "integration")
	app3Path := filepath.Join(rootPath, "app3")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --working-dir "+app3Path,
		&showStdout,
		&showStderr,
	)
	require.Error(t, err)
	// Verify that we fail because the dependency is not applied yet
	assert.Contains(t, err.Error(), "has not been applied yet")

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestTFDependencyOutputSkipOutputs(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "integration")
	emptyPath := filepath.Join(rootPath, "empty")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	// Test that even if the dependency (app1) is not applied, using skip_outputs will skip pulling the outputs so there
	// will be no errors.
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --working-dir "+emptyPath,
		&showStdout,
		&showStderr,
	)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestTFDependencyOutputSkipOutputsWithMockOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "mock-outputs")
	dependent3Path := filepath.Join(rootPath, "dependent3")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+dependent3Path,
		&showStdout,
		&showStderr,
	)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+dependent3Path,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 0", outputs["truth"].Value)

	// Now run --all apply so that the dependency is applied, and verify it still uses the mock output
	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+rootPath,
		&showStdout,
		&showStderr,
	)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+dependent3Path,
			&stdout,
			&stderr,
		),
	)

	outputs = map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 0", outputs["truth"].Value)
}

// Test that when you have a mock_output on a dependency, the dependency will use the mock as the output instead
// of erroring out.
func TestTFDependencyMockOutput(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "mock-outputs")
	dependent1Path := filepath.Join(rootPath, "dependent1")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+dependent1Path,
		&showStdout,
		&showStderr,
	)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+dependent1Path,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 0", outputs["truth"].Value)

	// Now run --all apply so that the dependency is applied, and verify it uses the dependency output
	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+rootPath,
		&showStdout,
		&showStderr,
	)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+dependent1Path,
			&stdout,
			&stderr,
		),
	)

	outputs = map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 42", outputs["truth"].Value)
}

// Test default behavior when mock_outputs_merge_with_state is not set. It should behave, as before this parameter was added
// It will fail on any command if the parent state is not applied, because the state of the parent exists and it already has an output
// but not the newly added output.
func TestTFDependencyMockOutputMergeWithStateDefault(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-with-state",
		"merge-with-state-default",
		"live",
	)
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --working-dir "+parentPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "plan stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify we have the default behavior if mock_outputs_merge_with_state is not set
	stdout.Reset()
	stderr.Reset()
	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --working-dir "+childPath,
		&stdout,
		&stderr,
	)
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
func TestTFDependencyMockOutputMergeWithStateFalse(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-with-state",
		"merge-with-state-false",
		"live",
	)
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --working-dir "+parentPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "plan stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify we have the default behavior if mock_outputs_merge_with_state is set to false
	stdout.Reset()
	stderr.Reset()
	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --working-dir "+childPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)
	// Verify that we fail because the dependency is not applied yet, and the new attribute is not available and in
	// this case, mocked outputs are not used.
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output2\"")

	helpers.LogBufferContentsLineByLine(t, stdout, "plan stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "plan stderr")
}

// Test when mock_outputs_merge_with_state is explicitly set to true.
// It will mock the newly added output from the parent as it was not already applied to the state.
func TestTFDependencyMockOutputMergeWithStateTrue(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-with-state",
		"merge-with-state-true",
		"live",
	)
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --working-dir "+parentPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "plan stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify mocked outputs are used if mock_outputs_merge_with_state is set to true and some output in the parent are not applied yet.
	stdout.Reset()
	stderr.Reset()
	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+childPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")
	// Now check the outputs to make sure they are as expected
	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+childPath,
			&stdout,
			&stderr,
		),
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
func TestTFDependencyMockOutputMergeWithStateTrueNotAllowed(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-with-state",
		"merge-with-state-true-validate-only",
		"live",
	)
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --working-dir "+parentPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "plan stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify mocked outputs are used if mock_outputs_merge_with_state is set to true with an allowed command and some
	// output in the parent are not applied yet.
	stdout.Reset()
	stderr.Reset()
	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt validate --non-interactive --working-dir "+childPath,
			&stdout,
			&stderr,
		),
	)

	// ... but not when an unallowed command is used
	require.Error(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+childPath,
			&stdout,
			&stderr,
		),
	)
}

// Test when mock_outputs_merge_with_state is explicitly set to true.
// Mock should not be used as the parent state was already fully applied.
func TestTFDependencyMockOutputMergeWithStateNoOverride(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-with-state",
		"merge-with-state-no-override",
		"live",
	)
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --working-dir "+parentPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "show stderr")

	// Verify mocked outputs are not used if mock_outputs_merge_with_state is set to true and all outputs in the parent have been applied.
	stdout.Reset()
	stderr.Reset()
	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+childPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)

	// Now check the outputs to make sure they are as expected
	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+childPath,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "value2", outputs["test_output2_from_parent"].Value)

	helpers.LogBufferContentsLineByLine(t, stdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "show stderr")
}

// Test when mock_outputs_merge_strategy_with_state or mock_outputs_merge_with_state is not set, the default is no_merge
func TestTFDependencyMockOutputMergeStrategyWithStateDefault(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-strategy-with-state",
		"merge-strategy-with-state-default",
		"live",
	)
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+childPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"This object does not have an attribute named \"test_output_list_string\"",
	)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_with_state = "false" that MergeStrategyType is set to no_merge
func TestTFDependencyMockOutputMergeStrategyWithStateCompatFalse(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-strategy-with-state",
		"merge-strategy-with-state-compat-false",
		"live",
	)
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+childPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"This object does not have an attribute named \"test_output_list_string\"",
	)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_with_state = "true" that MergeStrategyType is set to shallow
func TestTFDependencyMockOutputMergeStrategyWithStateCompatTrue(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-strategy-with-state",
		"merge-strategy-with-state-compat-true",
		"live",
	)
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+childPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+childPath,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	helpers.LogBufferContentsLineByLine(t, stdout, "output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(
		t,
		"map_root1_sub1_value",
		util.MustWalkTerraformOutput(
			outputs["test_output_map_map_string_from_parent"].Value,
			"map_root1",
			"map_root1_sub1",
			"value",
		),
	)
	assert.Nil(
		t,
		util.MustWalkTerraformOutput(
			outputs["test_output_map_map_string_from_parent"].Value,
			"not_in_state",
			"abc",
			"value",
		),
	)
	assert.Equal(
		t,
		"fake-list-data",
		util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"),
	)
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when both mock_outputs_merge_with_state and mock_outputs_merge_strategy_with_state are set, mock_outputs_merge_strategy_with_state is used
func TestTFDependencyMockOutputMergeStrategyWithStateCompatConflict(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-strategy-with-state",
		"merge-strategy-with-state-compat-true",
		"live",
	)
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+childPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+childPath,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	helpers.LogBufferContentsLineByLine(t, stdout, "output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(
		t,
		"map_root1_sub1_value",
		util.MustWalkTerraformOutput(
			outputs["test_output_map_map_string_from_parent"].Value,
			"map_root1",
			"map_root1_sub1",
			"value",
		),
	)
	assert.Nil(
		t,
		util.MustWalkTerraformOutput(
			outputs["test_output_map_map_string_from_parent"].Value,
			"not_in_state",
			"abc",
			"value",
		),
	)
	assert.Equal(
		t,
		"fake-list-data",
		util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"),
	)
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when mock_outputs_merge_strategy_with_state = "no_merge" that mocks are not merged into the current state
func TestTFDependencyMockOutputMergeStrategyWithStateNoMerge(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-strategy-with-state",
		"merge-strategy-with-state-no-merge",
		"live",
	)
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+childPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"This object does not have an attribute named \"test_output_list_string\"",
	)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_strategy_with_state = "shallow" that only top level outputs are merged.
// Lists or keys in existing maps will not be merged
func TestTFDependencyMockOutputMergeStrategyWithStateShallow(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-strategy-with-state",
		"merge-strategy-with-state-shallow",
		"live",
	)
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+childPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+childPath,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	helpers.LogBufferContentsLineByLine(t, stdout, "output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(
		t,
		"map_root1_sub1_value",
		util.MustWalkTerraformOutput(
			outputs["test_output_map_map_string_from_parent"].Value,
			"map_root1",
			"map_root1_sub1",
			"value",
		),
	)
	assert.Nil(
		t,
		util.MustWalkTerraformOutput(
			outputs["test_output_map_map_string_from_parent"].Value,
			"not_in_state",
			"abc",
			"value",
		),
	)
	assert.Equal(
		t,
		"fake-list-data",
		util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"),
	)
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when mock_outputs_merge_strategy_with_state = "deep" that the existing state is deeply merged into the mocks
// so that the existing state overwrites the mocks. This allows child modules to use new dependency outputs before the
// dependency has been applied
func TestTFDependencyMockOutputMergeStrategyWithStateDeepMapOnly(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(
		tmpEnvPath,
		testFixtureGetOutput,
		"mock-outputs-merge-strategy-with-state",
		"merge-strategy-with-state-deep-map-only",
		"live",
	)
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+childPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	helpers.LogBufferContentsLineByLine(t, stdout, "apply stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+childPath,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	helpers.LogBufferContentsLineByLine(t, stdout, "output stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "fake-abc", outputs["test_output2_from_parent"].Value)
	assert.Equal(
		t,
		"map_root1_sub1_value",
		util.MustWalkTerraformOutput(
			outputs["test_output_map_map_string_from_parent"].Value,
			"map_root1",
			"map_root1_sub1",
			"value",
		),
	)
	assert.Equal(
		t,
		"fake-abc",
		util.MustWalkTerraformOutput(
			outputs["test_output_map_map_string_from_parent"].Value,
			"not_in_state",
			"abc",
			"value",
		),
	)
	assert.Equal(
		t,
		"a",
		util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"),
	)
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test that when you have a mock_output on a dependency, the dependency will use the mock as the output instead
// of erroring out when running an allowed command.
func TestTFDependencyMockOutputRestricted(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "mock-outputs")
	dependent2Path := filepath.Join(rootPath, "dependent2")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+dependent2Path,
		&showStdout,
		&showStderr,
	)
	require.Error(t, err)
	// Verify that we fail because the dependency is not applied yet
	assert.Contains(t, err.Error(), "has not been applied yet")

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// Verify we can run when using one of the allowed commands
	showStdout.Reset()
	showStderr.Reset()
	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt validate --non-interactive --working-dir "+dependent2Path,
		&showStdout,
		&showStderr,
	)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// Verify that run --all validate works as well.
	showStdout.Reset()
	showStderr.Reset()
	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt run --all validate --non-interactive --working-dir "+rootPath,
		&showStdout,
		&showStderr,
	)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	showStdout.Reset()
	showStderr.Reset()
	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt run --all validate --non-interactive --working-dir "+rootPath,
		&showStdout,
		&showStderr,
	)
	require.NoError(t, err)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestTFDependencyOutputTypeConversion(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, ".")

	inputsPath := filepath.Join(tmpEnvPath, testFixtureInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "type-conversion")

	// First apply the inputs module
	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+inputsPath,
	)

	// Then apply the outputs module
	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
			&showStdout,
			&showStderr,
		),
	)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

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
	assert.InEpsilon(t, 42.0, outputs["number"].Value.(float64), 0.0000001)
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

// Regression testing for https://github.com/gruntwork-io/terragrunt/issues/1102: Ordering keys from
// maps to avoid random placements when terraform file is generated.
func TestTFOrderedMapOutputRegressions1102(t *testing.T) {
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
func TestTFDependencyOutputCycleHandling(t *testing.T) {
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
			rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "cycle", tc)
			fooPath := filepath.Join(rootPath, "foo")

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
func TestTFDependencyOutputRegression854(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "regression-854", "root")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+rootPath+
			" --filter '!{.}'",
	)
	require.NoError(t, err)
}

// Regression testing for bug where terragrunt output runs on dependency blocks are done in the terragrunt-cache for the
// child, not the parent.
func TestTFDependencyOutputCachePathBug(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "localstate", "live")

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

func TestTFDependencyOutputWithTerragruntSource(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "regression-1124", "live")
	modulePath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "regression-1124", "modules")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		fmt.Sprintf(
			"terragrunt run --all apply --non-interactive --working-dir %s --source %s",
			rootPath,
			modulePath,
		),
		&stdout,
		&stderr,
	)
	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

func TestTFRunAllWithSourceFlag(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRunAllSource)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRunAllSource)
	rootPath := filepath.Join(tmpEnvPath, testFixtureRunAllSource, "live")
	modulePath := filepath.Join(tmpEnvPath, testFixtureRunAllSource, "modules-marked")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all plan --non-interactive --working-dir %s --source %s",
			rootPath,
			modulePath,
		),
	)
	require.NoError(t, err)

	// When we fail to update the unit source location to the download dir correctly, we get an error about no configuration
	// files being present.
	assert.NotContains(t, stderr, "Error: No configuration files")

	unit1Path := filepath.Join(rootPath, "unit1")
	unit2Path := filepath.Join(rootPath, "unit2")

	// Find the cache directories for each unit
	unit1CacheDir := filepath.Join(unit1Path, helpers.TerragruntCache)
	unit2CacheDir := filepath.Join(unit2Path, helpers.TerragruntCache)

	var unit1MarkerPath, unit2MarkerPath string

	walkErr := filepath.WalkDir(
		unit1CacheDir,
		func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if d.Name() == "MODULE1_MARKER" {
				unit1MarkerPath = path
			}

			return nil
		},
	)
	require.NoError(t, walkErr)

	walkErr = filepath.WalkDir(
		unit2CacheDir,
		func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if d.Name() == "MODULE2_MARKER" {
				unit2MarkerPath = path
			}

			return nil
		},
	)
	require.NoError(t, walkErr)

	assert.NotEmpty(t, unit1MarkerPath)
	assert.NotEmpty(t, unit2MarkerPath)
}

func TestTFDependencyOutputWithHooks(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "regression-1273")
	depPath := filepath.Join(rootPath, "dep")
	mainPath := filepath.Join(rootPath, "main")

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	// The file should exist in cache dir (default hook behavior).
	assert.True(t, helpers.FileExistsInCache(t, depPath, "file.out"))
	assert.False(t, helpers.FileExistsInCache(t, mainPath, "file.out"))

	// Now delete file and run plain main again. It should NOT create file.out.
	cacheDir := helpers.FindCacheWorkingDir(t, depPath)
	require.NoError(t, os.Remove(filepath.Join(cacheDir, "file.out")))
	helpers.RunTerragrunt(t, "terragrunt plan --non-interactive --working-dir "+mainPath)
	assert.False(t, helpers.FileExistsInCache(t, depPath, "file.out"))
	assert.False(t, helpers.FileExistsInCache(t, mainPath, "file.out"))
}

func TestTFDeepDependencyOutputWithMock(t *testing.T) {
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

func TestTFDataDir(t *testing.T) {
	// Cannot be run in parallel with other tests as it modifies process' environment.
	helpers.CleanupTerraformFolder(t, testFixtureDirsPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDirsPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureDirsPath)

	t.Setenv("TF_DATA_DIR", filepath.Join(tmpEnvPath, "data_dir"))

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Initializing provider plugins")

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	assert.NotContains(t, stdout.String(), "Initializing provider plugins")
}

func TestTFReadTerragruntConfigWithDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureReadConfig)
	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, ".")

	inputsPath := filepath.Join(tmpEnvPath, testFixtureInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureReadConfig, "with_dependency")

	// First apply the inputs module
	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+inputsPath,
	)

	// Then apply the read config module
	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
			&showStdout,
			&showStderr,
		),
	)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

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
	assert.InEpsilon(t, 42.0, outputs["number"].Value.(float64), 0.0000001)
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

func TestTFReadTerragruntConfigFromDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureReadConfig)
	tmpEnvPath := helpers.CopyEnvironment(t, ".")
	rootPath := filepath.Join(tmpEnvPath, testFixtureReadConfig, "from_dependency")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt run --all apply --non-interactive --working-dir "+rootPath,
			&showStdout,
			&showStderr,
		),
	)

	helpers.LogBufferContentsLineByLine(t, showStdout, "show stdout")
	helpers.LogBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "hello world", outputs["bar"].Value)
}

func TestTFReadTerragruntConfigWithDefault(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReadConfig)
	helpers.CleanupTerraformFolder(t, filepath.Join(tmpEnvPath, testFixtureReadConfig))
	rootPath := filepath.Join(tmpEnvPath, testFixtureReadConfig, "with_default")

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	// check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "default value", outputs["data"].Value)
}

func TestTFReadTerragruntConfigWithOriginalTerragruntDir(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReadConfig)
	helpers.CleanupTerraformFolder(t, filepath.Join(tmpEnvPath, testFixtureReadConfig))
	rootPath := filepath.Join(tmpEnvPath, testFixtureReadConfig, "with_original_terragrunt_dir")

	rootPathAbs := filepath.Clean(rootPath)

	fooPathAbs := filepath.Join(rootPathAbs, "foo")
	depPathAbs := filepath.Join(rootPathAbs, "dep")

	// Run apply on the dependency module and make sure we get the outputs we expect
	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+depPathAbs,
	)

	depStdout := bytes.Buffer{}
	depStderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+depPathAbs,
			&depStdout,
			&depStderr,
		),
	)

	depOutputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(depStdout.Bytes(), &depOutputs))

	assert.Equal(t, depPathAbs, depOutputs["terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, depOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, depOutputs["bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, depOutputs["bar_original_terragrunt_dir"].Value)

	// Run apply on the root module and make sure we get the expected outputs
	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	rootStdout := bytes.Buffer{}
	rootStderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
			&rootStdout,
			&rootStderr,
		),
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
	helpers.RunTerragrunt(
		t,
		"terragrunt run --all  --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)

	runAllRootStdout := bytes.Buffer{}
	runAllRootStderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
			&runAllRootStdout,
			&runAllRootStderr,
		),
	)

	runAllRootOutputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(runAllRootStdout.Bytes(), &runAllRootOutputs))

	runAllDepStdout := bytes.Buffer{}
	runAllDepStderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+depPathAbs,
			&runAllDepStdout,
			&runAllDepStderr,
		),
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

func TestTFReadTerragruntConfigFull(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures")
	helpers.CleanupTerraformFolder(t, filepath.Join(tmpEnvPath, testFixtureReadConfig))
	rootPath := filepath.Join(tmpEnvPath, testFixtureReadConfig, "full")

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
	)

	// check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+rootPath,
			&stdout,
			&stderr,
		),
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
				"mutable":           nil,
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
	require.NoError(
		t,
		json.Unmarshal([]byte(outputs["remote_state"].Value.(string)), &remoteStateOut),
	)
	assert.Equal(
		t,
		map[string]any{
			"backend":                         "local",
			"disable_init":                    false,
			"disable_dependency_optimization": false,
			"generate": map[string]any{
				"path":      "backend.tf",
				"if_exists": "overwrite_terragrunt",
			},
			"config":     map[string]any{"path": "foo.tfstate"},
			"encryption": map[string]any{"key_provider": "foo"},
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

func TestTFTerragruntGenerateBlockSkipRemove(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(tmpEnvPath, testFixtureCodegenPath, "remove-file", "skip")

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	assert.FileExists(t, filepath.Join(generateTestCase, "backend.tf"))
}

func TestTFTerragruntGenerateBlockRemove(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(tmpEnvPath, testFixtureCodegenPath, "remove-file", "remove")

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	// With cache always used, the generate block removes files from the cache directory
	assert.False(t, helpers.FileExistsInCache(t, generateTestCase, "backend.tf"))
}

func TestTFTerragruntGenerateBlockRemoveTerragruntSuccess(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"remove-file",
		"remove_terragrunt",
	)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	// With cache always used, the generate block removes files from the cache directory
	assert.False(t, helpers.FileExistsInCache(t, generateTestCase, "backend.tf"))
}

func TestTFTerragruntGenerateBlockRemoveTerragruntFail(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"remove-file",
		"remove_terragrunt_error",
	)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	require.Error(t, err)

	var generateFileRemoveError codegen.GenerateFileRemoveError

	ok := errors.As(err, &generateFileRemoveError)
	assert.True(t, ok)

	assert.FileExists(t, filepath.Join(generateTestCase, "backend.tf"))
}

func TestTFTerragruntGenerateBlockSkip(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(tmpEnvPath, testFixtureCodegenPath, "generate-block", "skip")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)
	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	assert.False(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
}

func TestTFTerragruntGenerateBlockOverwrite(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"generate-block",
		"overwrite",
	)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, helpers.FileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTFTerragruntGenerateAttr(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-attr")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	text := "test-terragrunt-generate-attr-hello-world"

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --tf-forward-stdout --working-dir %s -var text=\"%s\"",
			generateTestCase,
			text,
		),
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, text)
}

func TestTFTerragruntGenerateBlockOverwriteTerragruntSuccess(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"generate-block",
		"overwrite_terragrunt",
	)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, helpers.FileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTFTerragruntGenerateBlockOverwriteTerragruntFail(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"generate-block",
		"overwrite_terragrunt_error",
	)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	var generateFileExistsError codegen.GenerateFileExistsError

	ok := errors.As(err, &generateFileExistsError)
	assert.True(t, ok)
}

func TestTFTerragruntGenerateBlockNestedInherit(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"generate-block",
		"nested",
		"child_inherit",
	)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	// If the state file was written as foo.tfstate, that means it inherited the config
	assert.True(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, helpers.FileIsInFolder(t, "bar.tfstate", generateTestCase))
	// Also check to make sure the child config generate block was included
	assert.True(t, helpers.FileIsInFolder(t, "random_file.txt", generateTestCase))
}

func TestTFTerragruntGenerateBlockNestedOverwrite(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"generate-block",
		"nested",
		"child_overwrite",
	)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	// If the state file was written as bar.tfstate, that means it overwrite the parent config
	assert.False(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.True(t, helpers.FileIsInFolder(t, "bar.tfstate", generateTestCase))
	// Also check to make sure the child config generate block was included
	assert.True(t, helpers.FileIsInFolder(t, "random_file.txt", generateTestCase))
}

func TestTFTerragruntGenerateBlockDisableSignature(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"generate-block",
		"disable-signature",
	)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+generateTestCase,
			&stdout,
			&stderr,
		),
	)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "Hello, World!", outputs["text"].Value)
}

func TestTFTerragruntGenerateBlockSameNameFail(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"generate-block",
		"same_name_error",
	)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt init --working-dir "+generateTestCase,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	var parsedError config.DuplicatedGenerateBlocksError

	ok := errors.As(err, &parsedError)
	assert.True(t, ok)
	assert.Len(t, parsedError.BlockName, 1)
	assert.Contains(t, parsedError.BlockName, "backend")
}

func TestTFTerragruntGenerateBlockSameNameIncludeFail(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"generate-block",
		"same_name_includes_error",
	)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt init --working-dir "+generateTestCase,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	var parsedError config.DuplicatedGenerateBlocksError

	ok := errors.As(err, &parsedError)
	assert.True(t, ok)
	assert.Len(t, parsedError.BlockName, 1)
	assert.Contains(t, parsedError.BlockName, "backend")
}

func TestTFTerragruntGenerateBlockMultipleSameNameFail(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"generate-block",
		"same_name_pair_error",
	)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt init --working-dir "+generateTestCase,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	var parsedError config.DuplicatedGenerateBlocksError

	ok := errors.As(err, &parsedError)
	assert.True(t, ok)
	assert.Len(t, parsedError.BlockName, 2)
	assert.Contains(t, parsedError.BlockName, "backend")
	assert.Contains(t, parsedError.BlockName, "backend2")
}

func TestTFTerragruntGenerateBlockDisable(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"generate-block",
		"disable",
	)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt init --working-dir "+generateTestCase,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	assert.False(t, helpers.FileIsInFolder(t, "data.txt", generateTestCase))
}

func TestTFTerragruntGenerateBlockEnable(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"generate-block",
		"enable",
	)
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt init --working-dir "+generateTestCase,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	assert.True(t, helpers.FileIsInFolder(t, "data.txt", generateTestCase))
}

func TestTFTerragruntRemoteStateCodegenGeneratesBackendBlock(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(tmpEnvPath, testFixtureCodegenPath, "remote-state", "base")

	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	// If the state file was written as foo.tfstate, that means it wrote out the local backend config.
	assert.True(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
}

func TestTFTerragruntRemoteStateCodegenPreservesAssumeRoleListCommas(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"remote-state",
		"s3-assume-role-lists",
	)

	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(
		t,
		"terragrunt init --non-interactive --working-dir "+generateTestCase+" -- -backend=false",
	)

	backendPath := filepath.Join(helpers.FindCacheWorkingDir(t, generateTestCase), "backend.tf")
	backendBytes, err := os.ReadFile(backendPath)
	require.NoError(t, err)

	backend := string(backendBytes)
	assert.Contains(
		t,
		backend,
		`policy_arns = ["arn:aws:iam::123456789342:policy/test-policy", "arn:aws:iam::123456789342:policy/other-policy"]`,
	)
	assert.Contains(t, backend, `transitive_tag_keys = ["Project", "ProjectSlug"]`)
}

func TestTFTerragruntRemoteStateCodegenOverwrites(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(
		tmpEnvPath,
		testFixtureCodegenPath,
		"remote-state",
		"overwrite",
	)

	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, helpers.FileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTFTerragruntRemoteStateCodegenErrorsIfExists(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(tmpEnvPath, testFixtureCodegenPath, "remote-state", "error")
	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	var generateFileExistsError codegen.GenerateFileExistsError

	ok := errors.As(err, &generateFileExistsError)
	assert.True(t, ok)
}

func TestTFTerragruntRemoteStateCodegenDoesNotGenerateWithSkip(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := filepath.Join(tmpEnvPath, testFixtureCodegenPath, "remote-state", "skip")

	helpers.CleanupTerraformFolder(t, generateTestCase)
	helpers.CleanupTerragruntFolder(t, generateTestCase)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+generateTestCase,
	)
	assert.False(t, helpers.FileIsInFolder(t, "foo.tfstate", generateTestCase))
}

// This function cannot be parallelized as it changes the global version.Version
//
//nolint:paralleltest
func TestTFTerragruntValidateAllWithVersionChecks(t *testing.T) {
	tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/version-check")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntVersionCommand(
		t,
		"v0.23.21",
		"terragrunt run --all validate --non-interactive --working-dir "+tmpEnvPath,
		&stdout,
		&stderr,
	)
	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

func TestTFTerragruntIncludeParentHclFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureIncludeParent)
	tmpEnvPath = path.Join(tmpEnvPath, testFixtureIncludeParent)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --log-level debug --all apply --non-interactive --working-dir "+tmpEnvPath,
	)
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
func TestTFTerragruntVersionConstraints(t *testing.T) {
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

			tmpTerragruntConfigPath := helpers.CreateTmpTerragruntConfigContent(
				t,
				tc.terragruntConstraint,
				config.DefaultTerragruntConfigPath,
			)

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

func TestTFReadTerragruntAuthProviderCmd(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "multiple-apps")
	appPath := filepath.Join(rootPath, "app1")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	helpers.ValidateAuthProviderScript(t, rootPath, mockAuthCmd)

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			`terragrunt run --all --non-interactive --working-dir %s --auth-provider-cmd %s -- apply -auto-approve`,
			rootPath,
			mockAuthCmd,
		),
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt output -json --working-dir %s --auth-provider-cmd %s",
			appPath,
			mockAuthCmd,
		),
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "app1-bar", outputs["foo-app1"].Value)
	assert.Equal(t, "app2-bar", outputs["foo-app2"].Value)
	assert.Equal(t, "app3-bar", outputs["foo-app3"].Value)
}

// TestTFReadTerragruntAuthProviderCmdEnvInLocalsRunAll verifies that
// auth-provider-cmd credentials are available during discovery queue
// construction (run --all). Without the fix in phase_parse.go, get_env()
// in locals cannot see env vars injected by --auth-provider-cmd because
// ObtainCredsForParsing is never called during the discovery parse phase.
func TestTFReadTerragruntAuthProviderCmdEnvInLocalsRunAll(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "env-in-locals")
	mockAuthCmd := filepath.Join(rootPath, "mock-auth-cmd.sh")

	helpers.RunTerragrunt(
		t, fmt.Sprintf(
			`terragrunt run --all --non-interactive --working-dir %s --auth-provider-cmd %s -- apply -auto-approve`,
			rootPath,
			mockAuthCmd,
		),
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t, fmt.Sprintf(
			"terragrunt run --non-interactive --working-dir %s --auth-provider-cmd %s -- output -json",
			rootPath,
			mockAuthCmd,
		),
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "from-auth-provider", outputs["secret"].Value)
}

func TestTFReadTerragruntAuthProviderCmdRunAllCallCountWithRacing(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "run-all-call-count")
	authProviderCmd := filepath.Join(rootPath, "auth-provider.sh")
	logPath := filepath.Join(rootPath, "calls.jsonl")

	helpers.ValidateAuthProviderScript(t, rootPath, authProviderCmd)
	require.NoError(t, os.Remove(logPath), "auth-provider.sh should have created %s", logPath)

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --all apply --non-interactive "+
				"--working-dir %s --auth-provider-cmd %s",
			rootPath,
			authProviderCmd,
		),
	)

	logBytes, err := os.ReadFile(logPath)
	require.NoError(t, err)

	type authCall struct {
		Timestamp  string `json:"ts"`
		WorkingDir string `json:"working_dir"`
		PID        int    `json:"pid"`
	}

	lines := strings.Split(strings.TrimRight(string(logBytes), "\n"), "\n")
	calls := make([]authCall, 0, len(lines))

	for i, line := range lines {
		var call authCall

		require.NoErrorf(t, json.Unmarshal([]byte(line), &call), "line %d: %q", i+1, line)

		calls = append(calls, call)
	}

	t.Logf("auth-provider-cmd invocations:\n%s", string(logBytes))

	// Five invocations are expected, in this order:
	//
	//   1. Discovery relationship phase parses `dep`.
	//   2. Discovery relationship phase parses `dependent`.
	//   3. Runner pool task parses `dep`for apply.
	//   4. Runner pool task parses `dependent` for apply.
	//   5. While `dependent`'s HCL is being decoded, the `dependency.dep`
	//      block fetches `dep`'s outputs, which requires parsing `dep` again.
	//
	assert.Len(t, calls, 5)

	for _, call := range calls {
		assert.NotContains(t, call.WorkingDir, ".terragrunt-cache")
	}
}

func TestTFNoDiscoveryAuthProviderCmdSkipsDiscoveryAuthWithRacing(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "run-all-call-count")
	authProviderCmd := filepath.Join(rootPath, "auth-provider.sh")
	logPath := filepath.Join(rootPath, "calls.jsonl")

	helpers.ValidateAuthProviderScript(t, rootPath, authProviderCmd)
	require.NoError(t, os.Remove(logPath), "auth-provider.sh should have created %s", logPath)

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --all apply --non-interactive "+
				"--no-discovery-auth-provider-cmd "+
				"--working-dir %s --auth-provider-cmd %s",
			rootPath,
			authProviderCmd,
		),
	)

	logBytes, err := os.ReadFile(logPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(string(logBytes), "\n"), "\n")

	t.Logf("auth-provider-cmd invocations:\n%s", string(logBytes))

	// Without --no-discovery-auth-provider-cmd this fixture produces five
	// invocations (see TestTFReadTerragruntAuthProviderCmdRunAllCallCountWithRacing).
	// The flag skips the two discovery-relationship-phase invocations, leaving
	// the three runtime invocations from the runner pool and the dependency
	// outputs fetch.
	assert.Len(t, lines, 3)
}

func TestTFIamRolesLoadingFromDifferentModules(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureIamRolesMultipleModules)

	// Invoke terragrunt and verify used IAM roles for each dependency
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt init --debugreset --log-level debug --working-dir "+testFixtureIamRolesMultipleModules,
	)

	// Taking all outputs in one string
	output := fmt.Sprintf("%v %v %v", stderr, stdout, err.Error())

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
func TestTFTerragruntVersionConstraintsPartialParse(t *testing.T) {
	fixturePath := "fixtures/partial-parse/terragrunt-version-constraint"
	helpers.CleanupTerragruntFolder(t, fixturePath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntVersionCommand(
		t,
		"0.21.23",
		"terragrunt apply -auto-approve --non-interactive --working-dir "+fixturePath,
		&stdout,
		&stderr,
	)
	helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
	helpers.LogBufferContentsLineByLine(t, stderr, "stderr")

	require.Error(t, err)

	var invalidVersionError run.InvalidTerragruntVersion

	ok := errors.As(err, &invalidVersionError)
	assert.True(t, ok)
}

func TestTFLogFailingDependencies(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testFixtureBrokenDependency, "app")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --working-dir %s --log-level trace",
			path,
		),
	)
	require.Error(t, err)

	// Check that the error output contains terraform/tofu error details
	assert.Contains(t, stderr, "Getting output of dependency ../dependency/terragrunt.hcl")
	assert.Contains(t, stderr, "Error: Failed to download module")
}

func TestTFDependenciesOptimisation(t *testing.T) {
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependenciesOptimisation)
	rootPath := filepath.Join(tmpEnvPath, testFixtureDependenciesOptimisation)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve",
	)
	require.NoError(t, err)

	assert.NotContains( // Check that we're getting a warning for usage of deprecated functionality.
		t,
		stderr,
		"Reading inputs from dependencies has been deprecated and will be removed in a future version of Terragrunt. If a value in a dependency is needed, use dependency outputs instead.",
	)

	moduleC := filepath.Join(tmpEnvPath, testFixtureDependenciesOptimisation, "module-c")

	t.Setenv("TERRAGRUNT_STRICT_CONTROL", "skip-dependencies-inputs")
	_, stderr, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+moduleC,
	)
	require.NoError(t, err)

	// checking that dependencies optimisation is working and outputs from module-a are not retrieved
	assert.NotContains(t, stderr, "Retrieved output from ../module-a/terragrunt.hcl")
}

func TestTFNoMultipleInitsWithoutSourceChange(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.NewGitServer(t).RenderFixture(testFixtureDownload)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureStdout)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+testPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	// providers initialization during first plan
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+testPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	// no initialization expected for second plan run
	// https://github.com/gruntwork-io/terragrunt/issues/1921
	assert.Equal(t, 0, strings.Count(stdout.String(), "has been successfully initialized!"))
}

func TestTFAutoInitWhenSourceIsChanged(t *testing.T) {
	t.Parallel()

	mirror := helpers.NewGitServer(t)
	tmpEnvPath := mirror.RenderFixture(testFixtureDownload)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureAutoInit)

	terragruntHcl := filepath.Join(testPath, "terragrunt.hcl")

	contents, err := vfs.ReadFileAsString(vfs.NewOSFS(), terragruntHcl)
	if err != nil {
		require.NoError(t, err)
	}

	updatedHcl := strings.ReplaceAll(contents, "__TAG_VALUE__", "v0.78.4")
	require.NoError(t, os.WriteFile(terragruntHcl, []byte(updatedHcl), 0o444))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+testPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	// providers initialization during first plan
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))

	updatedHcl = strings.ReplaceAll(contents, "__TAG_VALUE__", "v0.79.0")
	require.NoError(t, os.WriteFile(terragruntHcl, []byte(updatedHcl), 0o444))

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --tf-forward-stdout --working-dir "+testPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	// auto initialization when source is changed
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))
}

func TestTFNoColor(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoColor)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureNoColor)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan -no-color --tf-forward-stdout --working-dir "+testPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	// providers initialization during first plan
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))

	assert.NotContains(t, stdout.String(), "\x1b")
}

func TestTFTerragruntValidateModulePrefix(t *testing.T) {
	t.Parallel()

	fixturePath := testFixtureIncludeParent
	helpers.CleanupTerraformFolder(t, fixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, fixturePath)
	rootPath := filepath.Join(tmpEnvPath, fixturePath)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all validate --tf-forward-stdout --non-interactive --working-dir "+rootPath,
	)
}

func TestTFInitFailureModulePrefix(t *testing.T) {
	t.Parallel()

	initTestCase := testFixtureInitError

	helpers.CleanupTerraformFolder(t, initTestCase)
	helpers.CleanupTerragruntFolder(t, initTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.Error(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt init -no-color --non-interactive --working-dir "+initTestCase,
			&stdout,
			&stderr,
		),
	)
	helpers.LogBufferContentsLineByLine(t, stderr, "init")
	// Check for terraform error in structured log format
	assert.Contains(t, stderr.String(), "level=stderr")
}

func TestTFDependencyOutputModulePrefix(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "integration")

	helpers.RunTerragrunt(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath)

	// verify expected output 42
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	app3Path := filepath.Join(rootPath, "app3")
	require.NoError(
		t,
		helpers.RunTerragruntCommand(
			t,
			"terragrunt output -no-color -json --non-interactive --working-dir "+app3Path,
			&stdout,
			&stderr,
		),
	)
	// validate that output is valid json
	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, 42, int(outputs["z"].Value.(float64)))
}

func TestTFExplainingMissingCredentials(t *testing.T) {
	// no parallel because we need to set env vars
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/tmp/not-existing-creds-46521694")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInitError)
	initTestCase := filepath.Join(tmpEnvPath, testFixtureInitError)

	helpers.CleanupTerraformFolder(t, initTestCase)
	helpers.CleanupTerragruntFolder(t, initTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt init -no-color --tf-forward-stdout --non-interactive --working-dir "+initTestCase,
		&stdout,
		&stderr,
	)
	explanation := shell.ExplainError(err)
	assert.Contains(t, explanation, "Missing AWS credentials")
}

func TestTFModulePathInPlanErrorMessage(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureModulePathError)
	rootPath := filepath.Join(tmpEnvPath, testFixtureModulePathError, "app")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan -no-color --non-interactive --working-dir "+rootPath,
	)
	require.Error(t, err)
	output := stdout + "\n" + stderr + "\n" + err.Error() + "\n"

	assert.Contains(t, output, "resolving dependency")
}

func TestTFModulePathInRunAllPlanErrorMessage(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureModulePathError)
	rootPath := filepath.Join(tmpEnvPath, testFixtureModulePathError)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --report-file report.json --working-dir "+rootPath+" -- plan -no-color",
	)
	require.Error(t, err)

	// catch "Run failed" message printed in case of error in apply of units
	assert.Contains(t, stderr, "Run failed")

	runs := helpers.ReadReport(t, rootPath, "report.json")
	assert.NotNil(t, runs.FindByName("d1"))
}

func TestTFInitSkipCache(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureInitCache)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInitCache)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInitCache, "app")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --log-level debug --tf-forward-stdout --working-dir "+rootPath,
	)
	require.NoError(t, err)

	// verify that init was invoked
	assert.Contains(t, stdout, "has been successfully initialized!")
	assert.Contains(t, stderr, "Running command: "+wrappedBinary(t.Context())+" init")

	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --log-level debug --tf-forward-stdout --working-dir "+rootPath,
	)
	require.NoError(t, err)

	// verify that init wasn't invoked second time since cache directories are ignored
	assert.NotContains(t, stdout, "has been successfully initialized!")
	assert.NotContains(t, stderr, "Running command: "+wrappedBinary(t.Context())+" init")

	// verify that after adding new file, init is executed
	tfFile := filepath.Join(tmpEnvPath, testFixtureInitCache, "app", "project.tf")
	if err := os.WriteFile(tfFile, []byte(""), 0o644); err != nil {
		t.Fatalf("Error writing new Terraform file to %s: %v", tfFile, err)
	}

	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --log-level debug --tf-forward-stdout --working-dir "+rootPath,
	)
	require.NoError(t, err)

	// verify that init was invoked
	assert.Contains(t, stdout, "has been successfully initialized!")
	assert.Contains(t, stderr, "Running command: "+wrappedBinary(t.Context())+" init")
}

func TestTFTerragruntNoWarningLocalPath(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDisabledPath)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureDisabledPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply --non-interactive --working-dir "+testPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	assert.NotContains(t, stderr.String(), "No double-slash (//) found in source URL")
}

func TestTFTerragruntDisabledDependency(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDisabledModule)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureDisabledModule, "app")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all plan --non-interactive --working-dir "+testPath,
	)
	require.NoError(t, err)

	// check that only enabled dependencies are evaluated
	assert.Contains(t, stderr, "unit-without-enabled")
	assert.Contains(t, stderr, "unit-enabled")
	assert.NotContains(t, stderr, "unit-disabled")
}

func TestTFTerragruntHandleEmptyStateFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureEmptyState)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureEmptyState)

	helpers.CreateEmptyStateFile(t, testPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+testPath,
	)
}

func TestTFTerragruntCommandsThatNeedInput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureCommandsThatNeedInput)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureCommandsThatNeedInput)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply --non-interactive --tf-forward-stdout --working-dir "+testPath,
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Apply complete")
}

func TestTFTerragruntSkipDependenciesWithSkipFlag(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureSkipDependencies)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureSkipDependencies)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt run --all apply --no-color --non-interactive --working-dir "+testPath,
		&stdout,
		&stderr,
	)
	require.NoError(t, err)

	output := fmt.Sprintf("%s %s", stderr.String(), stdout.String())

	assert.NotContains(t, output, "Error reading partial config for dependency")
	assert.NotContains(t, output, "Call to function \"find_in_parent_folders\" failed")
	assert.NotContains(t, output, "ParentFileNotFoundError")

	// Check that units were excluded at stack level (shown in Run Summary)
	assert.Contains(t, output, "Excluded")
	// check that no test_file.txt was created in module directory
	_, err = os.Stat(
		filepath.Join(tmpEnvPath, testFixtureSkipDependencies, "first", "test_file.txt"),
	)
	require.Error(t, err)
	_, err = os.Stat(
		filepath.Join(tmpEnvPath, testFixtureSkipDependencies, "second", "test_file.txt"),
	)
	require.Error(t, err)
}

func TestTFStorePlanFilesRunAllPlanApply(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := helpers.TmpDirWOSymlinks(t)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureOutDir)
	dependencyPath := filepath.Join(tmpEnvPath, testFixtureOutDir, "dependency")

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --working-dir %s --out-dir %s",
			dependencyPath,
			tmpDir,
		),
	)

	// run plan with output directory
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all plan --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)

	// verify that tfplan files are created in the tmpDir, 2 files
	list, err := findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all apply --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)
}

func TestTFStorePlanFilesRunAllPlanApplyRelativePath(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureOutDir)

	dependencyPath := filepath.Join(tmpEnvPath, testFixtureOutDir, "dependency")
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --working-dir %s --out-dir %s",
			dependencyPath,
			testPath,
		),
	)

	// run plan with output directory
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all plan --non-interactive --working-dir %s --out-dir %s",
			testPath,
			"test",
		),
	)
	require.NoError(t, err)

	outDir := filepath.Join(testPath, "test")

	// verify that tfplan files are created in the tmpDir, 2 files
	list, err := findFilesWithExtension(outDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all apply --non-interactive --working-dir %s --out-dir test",
			testPath,
		),
	)
	require.NoError(t, err)
}

// TestTFRunAllApplyWithCustomPlanFileName tests issue #5409
// When using `run --all apply` with a plan file without .tfplan extension,
// the plan file should be moved to the end of args, after flags.
func TestTFRunAllApplyWithCustomPlanFileName(t *testing.T) {
	t.Parallel()

	// Reuse existing fixture
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureOutDir)
	dependencyPath := filepath.Join(tmpEnvPath, testFixtureOutDir, "dependency")

	// Apply dependency first (required by app)
	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+dependencyPath,
	)

	// Step 1: Create plan with custom name (no .tfplan extension)
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt run --all plan --non-interactive --working-dir %s -- -out=customplan",
		testPath,
	))
	require.NoError(t, err)

	// Step 2: Apply using the custom plan file name
	// This should fail before fix with "Too many command line arguments"
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt run --all apply --non-interactive --working-dir %s -- customplan",
		testPath,
	))

	// Assertions
	require.NoError(t, err, "Apply should succeed")

	output := stdout + stderr
	require.NotContains(t, output, "Too many command line arguments")
	require.NotContains(t, output, "Expected at most one positional argument")
}

func TestTFStorePlanFilesJsonRelativePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args string
	}{
		{"run --all plan --non-interactive --working-dir %s --out-dir test --json-out-dir json"},
		{"run plan --all --non-interactive --working-dir %s --out-dir test --json-out-dir json"},
		{"run plan -a --non-interactive --working-dir %s --out-dir test --json-out-dir json"},
		{"run --all --non-interactive --working-dir %s --out-dir test --json-out-dir json -- plan"},
	}

	for _, tc := range testCases {
		t.Run("terragrunt args: "+tc.args, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
			helpers.CleanupTerraformFolder(t, tmpEnvPath)
			testPath := filepath.Join(tmpEnvPath, testFixtureOutDir)

			// run plan with output directory
			_, _, err := helpers.RunTerragruntCommandWithOutput(
				t,
				fmt.Sprintf("terragrunt "+tc.args, testPath),
			)
			require.NoError(t, err)

			// verify that tfplan files are created in the tmpDir, 2 files
			outDir := filepath.Join(testPath, "test")
			list, err := findFilesWithExtension(outDir, ".tfplan")
			require.NoError(t, err)
			assert.Len(t, list, 2)

			// verify that json files are create
			jsonDir := filepath.Join(testPath, "json")
			listJSON, err := findFilesWithExtension(jsonDir, ".json")
			require.NoError(t, err)
			assert.Len(t, listJSON, 2)
		})
	}
}

func TestTFPlanJsonFilesRunAll(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := helpers.TmpDirWOSymlinks(t)
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

func TestTFPlanJsonPlanBinaryRunAll(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := helpers.TmpDirWOSymlinks(t)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureOutDir)

	dependencyPath := filepath.Join(tmpEnvPath, testFixtureOutDir, "dependency")
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --working-dir %s --out-dir %s",
			dependencyPath,
			tmpDir,
		),
	)

	// run plan with output directory
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all plan --non-interactive --working-dir %s --json-out-dir %s --out-dir %s",
			testPath,
			tmpDir,
			tmpDir,
		),
	)
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

func TestTFTerragruntRunAllPlanAndShow(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := helpers.TmpDirWOSymlinks(t)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureOutDir)

	dependencyPath := filepath.Join(tmpEnvPath, testFixtureOutDir, "dependency")
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --working-dir %s --out-dir %s",
			dependencyPath,
			tmpDir,
		),
	)

	// run plan and apply
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all plan --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all apply --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)

	// run new plan and show
	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all plan --non-interactive --working-dir %s --out-dir %s",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all show --non-interactive --tf-forward-stdout --working-dir %s --out-dir %s -no-color",
			testPath,
			tmpDir,
		),
	)
	require.NoError(t, err)

	// Verify that output contains the plan and not plain the actual state output
	assert.Contains(t, stdout, "No changes. Your infrastructure matches the configuration.")
}

func TestTFLogFormatJSONOutput(t *testing.T) {
	t.Parallel()

	mirror := helpers.NewGitServer(t)
	tmpEnvPath := mirror.RenderFixture(testFixtureNotExistingSource)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureNotExistingSource)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply --log-format=json --non-interactive --working-dir "+testPath,
	)
	require.Error(t, err)

	// for windows OS
	output := bytes.ReplaceAll([]byte(stderr), []byte("\r\n"), []byte("\n"))

	multipeJSONs := bytes.Split(output, []byte("\n"))

	msgs := make([]string, 0, len(multipeJSONs))

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

	assert.Contains(
		t,
		strings.Join(msgs, ""),
		"Downloading Terraform configurations from git::"+mirror.URL,
	)
	assert.Contains(t, strings.Join(msgs, ""), "ref=v0.83.2")
}

func TestTFTerragruntOutputFromDependencyLogsJson(t *testing.T) {
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
			rootTerragruntPath := filepath.Join(tmpEnvPath, testFixtureDependencyOutput)
			// apply dependency first
			dependencyTerragruntConfigPath := filepath.Join(rootTerragruntPath, "dependency")
			_, _, err := helpers.RunTerragruntCommandWithOutput(
				t,
				fmt.Sprintf(
					"terragrunt apply -auto-approve --non-interactive --working-dir %s ",
					dependencyTerragruntConfigPath,
				),
			)
			require.NoError(t, err)

			appTerragruntConfigPath := filepath.Join(rootTerragruntPath, "app")
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				fmt.Sprintf(
					"terragrunt plan --non-interactive --working-dir %s %s",
					appTerragruntConfigPath,
					tc.arg,
				),
			)
			require.NoError(t, err)

			output := fmt.Sprintf("%s %s", stderr, stdout)
			assert.NotContains(t, output, "invalid character")
		})
	}
}

func TestTFTerragruntJsonPlanJsonOutput(t *testing.T) {
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
			tmpDir := helpers.TmpDirWOSymlinks(t)
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

func TestTFTerragruntTerraformOutputJson(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInitError)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureInitError)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply --no-color --log-format=json --non-interactive --working-dir "+testPath,
	)
	require.Error(t, err)

	// Sometimes, this is the error returned by AWS.
	if !strings.Contains(
		stderr,
		"Error: Failed to get existing workspaces: operation error S3: ListObjectsV2, https response error StatusCode: 301",
	) {
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

func TestTFLogStreaming(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogStreaming)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureLogStreaming)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --working-dir "+testPath+" apply",
	)
	require.NoError(t, err)

	for _, unit := range []string{"unit1", "unit2"} {
		// Find the timestamps for the first and second log entries for this unit
		firstTimestamp := time.Time{}
		secondTimestamp := time.Time{}

		for line := range strings.SplitSeq(stdout, "\n") {
			if strings.Contains(line, unit) {
				if !strings.Contains(line, "(local-exec): sleeping...") &&
					!strings.Contains(line, "(local-exec): done sleeping") {
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
		require.GreaterOrEqualf(
			t,
			secondTimestamp.Sub(firstTimestamp),
			1*time.Second,
			"Second log entry for unit %s is not at least 1 second after the first log entry",
			unit,
		)
	}
}

func TestTFLogFormatBare(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureEmptyState)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureEmptyState)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt init --log-format=bare --no-color --non-interactive --working-dir "+testPath,
	)
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
	testPath := filepath.Join(tmpEnvPath, testFixtureEphemeralInputs)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --working-dir "+testPath,
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Plan: 1 to add, 0 to change, 0 to destroy")

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply --auto-approve --non-interactive --working-dir "+testPath,
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed")
}

func TestTFMixedStackConfigIgnored(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMixedConfig)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureMixedConfig)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --working-dir "+testPath+" -- apply",
	)
	require.NoError(t, err)
	require.NotContains(t, stderr, "Error: Unsupported block type")
	require.NotContains(t, stderr, "Blocks of type \"unit\" are not expected here")
}

func TestTFDiscoveryDoesntResolveOutputs(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	depDir := filepath.Join(tmpDir, "dep")
	err := os.MkdirAll(depDir, 0o755)
	require.NoError(t, err)

	mainDir := filepath.Join(tmpDir, "main")
	err = os.MkdirAll(mainDir, 0o755)
	require.NoError(t, err)

	depConfig := `
terraform {
  source = "."
}
`
	err = os.WriteFile(filepath.Join(depDir, "terragrunt.hcl"), []byte(depConfig), 0o644)
	require.NoError(t, err)

	depTerraform := `
output "value" {
  value = "hello from dependency"
}
`
	err = os.WriteFile(filepath.Join(depDir, "main.tf"), []byte(depTerraform), 0o644)
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
	err = os.WriteFile(filepath.Join(mainDir, "terragrunt.hcl"), []byte(mainConfig), 0o644)
	require.NoError(t, err)

	mainTerraform := `
variable "dep_value" {
  type = string
}

output "result" {
  value = var.dep_value
}
`
	err = os.WriteFile(filepath.Join(mainDir, "main.tf"), []byte(mainTerraform), 0o644)
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+depDir,
	)
	require.NoError(t, err)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -no-color -json --non-interactive --working-dir "+depDir,
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "hello from dependency")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+tmpDir,
	)
	require.NoError(t, err)

	assert.NotEmpty(t, stdout)
	assert.NotEmpty(t, stderr)

	assert.NotContains(
		t,
		stderr,
		"that has no outputs, but mock outputs provided and returning those in dependency output",
	)

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -no-color -json --non-interactive --working-dir "+mainDir,
	)
	require.NoError(t, err)

	assert.Contains(t, stdout, "hello from dependency")
}

func TestTFExternalDependenciesAreResolved(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	depDir := filepath.Join(tmpDir, "dep")
	err := os.MkdirAll(depDir, 0o755)
	require.NoError(t, err)

	mainDir := filepath.Join(tmpDir, "main")
	err = os.MkdirAll(mainDir, 0o755)
	require.NoError(t, err)

	depConfig := `
terraform {
  source = "."
}
`
	err = os.WriteFile(filepath.Join(depDir, "terragrunt.hcl"), []byte(depConfig), 0o644)
	require.NoError(t, err)

	depTerraform := `
output "value" {
  value = "hello from dependency"
}
`
	err = os.WriteFile(filepath.Join(depDir, "main.tf"), []byte(depTerraform), 0o644)
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
	err = os.WriteFile(filepath.Join(mainDir, "terragrunt.hcl"), []byte(mainConfig), 0o644)
	require.NoError(t, err)

	mainTerraform := `
variable "dep_value" {
  type = string
}

output "result" {
  value = var.dep_value
}
`
	err = os.WriteFile(filepath.Join(mainDir, "main.tf"), []byte(mainTerraform), 0o644)
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

func TestTFRunAllDetectsHiddenDirectories(t *testing.T) {
	t.Parallel()
	rootPath := helpers.CopyEnvironment(t, hiddenRunAllFixturePath, ".cloud/**")
	modulePath := filepath.Join(rootPath, hiddenRunAllFixturePath)
	helpers.CleanupTerraformFolder(t, modulePath)

	reportFile := filepath.Join(modulePath, helpers.ReportFile)

	// Expect Terragrunt to discover modules under .cloud directory
	command := fmt.Sprintf(
		"terragrunt run --all plan --non-interactive --working-dir %s --report-file %s --report-format json",
		modulePath,
		reportFile,
	)
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, command)
	require.NoError(t, err)

	// Parse the report file to verify the correct units ran
	runs, err := report.ParseJSONRunsFromFile(reportFile)
	require.NoError(t, err, "Should be able to parse JSON report")

	runNames := runs.Names()

	// Verify both hidden directories were discovered and executed
	app1Run := runs.FindByName(".cloud/terraform/app1")
	require.NotNil(
		t,
		app1Run,
		"Expected .cloud/terraform/app1 unit to be in report. Found: %v",
		runNames,
	)

	app2Run := runs.FindByName(".cloud/terraform/app2")
	require.NotNil(
		t,
		app2Run,
		"Expected .cloud/terraform/app2 unit to be in report. Found: %v",
		runNames,
	)
}

func TestTFNoColorDependency(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoColorDependency)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureNoColorDependency)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run plan -no-color --tf-forward-stdout --working-dir "+testPath,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(stdout, "has been successfully initialized!"))

	// check that no ANSI codes are printed
	assert.NotContains(t, stderr, "\x1b")
	assert.NotContains(t, stdout, "\x1b")
}

// TestTFTerragruntPassNullValues verifies that terragrunt can pass null values to
// Terraform variables. This is a regression test for:
// https://github.com/gruntwork-io/terragrunt/issues/5452
func TestTFTerragruntPassNullValues(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNullValue)
	testPath := filepath.Join(tmpEnvPath, testFixtureNullValue)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+testPath,
	)
	require.NoError(t, err)

	nullVarsFile := filepath.Join(
		testPath,
		".terragrunt-cache",
		"*",
		"*",
		".terragrunt-null-vars.auto.tfvars.json",
	)
	matches, err := filepath.Glob(nullVarsFile)
	require.NoError(t, err)
	assert.Empty(t, matches, "null vars file should be removed after execution")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -json --non-interactive --working-dir "+testPath,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	// output1 should not be present because OpenTofu/Terraform omit
	// null-valued outputs from JSON output
	_, ok := outputs["output1"]
	assert.False(t, ok, "expected output1 to not be present since it has a null value")

	output2, ok := outputs["output2"]
	require.True(t, ok, "expected output2 to be present")
	assert.Equal(t, "variable 2", output2.Value)
}
