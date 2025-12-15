package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/cli/commands"
	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl"
	hclformat "github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"

	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/config"
	clipkg "github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var defaultLogLevel = log.DebugLevel

func TestParseTerragruntOptionsFromArgs(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	workingDir = filepath.ToSlash(workingDir)

	testCases := []struct {
		expectedErr     error
		expectedOptions *options.TerragruntOptions
		args            []string
	}{
		{
			args:            []string{"plan"},
			expectedOptions: mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, false, defaultLogLevel, false),
		},

		{
			args:            []string{"plan", "bar"},
			expectedOptions: mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan", "bar"}, false, "", false, false, defaultLogLevel, false),
		},

		{
			args:            []string{"--foo", "--bar"},
			expectedOptions: mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"-foo", "-bar"}, false, "", false, false, defaultLogLevel, false),
			expectedErr:     clipkg.UndefinedFlagError("foo"),
		},

		{
			args:            []string{"--foo", "apply", "--bar"},
			expectedOptions: mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"apply", "-foo", "-bar"}, false, "", false, false, defaultLogLevel, false),
			expectedErr:     clipkg.UndefinedFlagError("foo"),
		},

		{
			args:            []string{doubleDashed(global.NonInteractiveFlagName)},
			expectedOptions: mockOptions(t, "", "", []string{}, true, "", false, false, defaultLogLevel, false),
		},

		{
			args:            []string{"apply", doubleDashed(shared.QueueIncludeExternalFlagName)},
			expectedOptions: mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"apply"}, false, "", false, true, defaultLogLevel, false),
		},

		{
			args:            []string{"plan", doubleDashed(run.ConfigFlagName), "/some/path/" + config.DefaultTerragruntConfigPath},
			expectedOptions: mockOptions(t, "/some/path/"+config.DefaultTerragruntConfigPath, workingDir, []string{"plan"}, false, "", false, false, defaultLogLevel, false),
		},

		{
			args:            []string{"plan", doubleDashed(global.WorkingDirFlagName), "/some/path"},
			expectedOptions: mockOptions(t, util.JoinPath("/some/path", config.DefaultTerragruntConfigPath), "/some/path", []string{"plan"}, false, "", false, false, defaultLogLevel, false),
		},

		{
			args:            []string{"plan", doubleDashed(run.SourceFlagName), "/some/path"},
			expectedOptions: mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "/some/path", false, false, defaultLogLevel, false),
		},

		{
			args:            []string{"plan", doubleDashed(run.SourceMapFlagName), "git::git@github.com:one/gw-terraform-aws-vpc.git=git::git@github.com:two/test.git?ref=FEATURE"},
			expectedOptions: mockOptionsWithSourceMap(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, map[string]string{"git::git@github.com:one/gw-terraform-aws-vpc.git": "git::git@github.com:two/test.git?ref=FEATURE"}),
		},

		{
			args:            []string{"plan", doubleDashed(shared.QueueIgnoreErrorsFlagName)},
			expectedOptions: mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", true, false, defaultLogLevel, false),
		},

		{
			args:            []string{"plan", doubleDashed(shared.QueueExcludeExternalFlagName)},
			expectedOptions: mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, false, defaultLogLevel, false),
		},

		{
			args:            []string{"plan", doubleDashed(run.IAMAssumeRoleFlagName), "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"},
			expectedOptions: mockOptionsWithIamRole(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"),
		},

		{
			args:            []string{"plan", doubleDashed(run.IAMAssumeRoleDurationFlagName), "36000"},
			expectedOptions: mockOptionsWithIamAssumeRoleDuration(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, 36000),
		},

		{
			args:            []string{"plan", doubleDashed(run.IAMAssumeRoleSessionNameFlagName), "terragrunt-iam-role-session-name"},
			expectedOptions: mockOptionsWithIamAssumeRoleSessionName(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, "terragrunt-iam-role-session-name"),
		},

		{
			args:            []string{"plan", doubleDashed(run.IAMAssumeRoleWebIdentityTokenFlagName), "web-identity-token"},
			expectedOptions: mockOptionsWithIamWebIdentityToken(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, "web-identity-token"),
		},

		{
			args:            []string{"plan", doubleDashed(run.ConfigFlagName), "/some/path/" + config.DefaultTerragruntConfigPath, "-non-interactive"},
			expectedOptions: mockOptions(t, "/some/path/"+config.DefaultTerragruntConfigPath, workingDir, []string{"plan"}, true, "", false, false, defaultLogLevel, false),
		},

		{
			args:            []string{"plan", doubleDashed(run.ConfigFlagName), "/some/path/" + config.DefaultTerragruntConfigPath, "bar", doubleDashed(global.NonInteractiveFlagName), "--baz", doubleDashed(global.WorkingDirFlagName), "/some/path", doubleDashed(run.SourceFlagName), "github.com/foo/bar//baz?ref=1.0.3"},
			expectedOptions: mockOptions(t, "/some/path/"+config.DefaultTerragruntConfigPath, "/some/path", []string{"plan", "bar", "-baz"}, true, "github.com/foo/bar//baz?ref=1.0.3", false, false, defaultLogLevel, false),
		},

		// Adding the --terragrunt-log-level flag should result in DebugLevel configured
		{
			args:            []string{"plan", doubleDashed(global.LogLevelFlagName), "debug"},
			expectedOptions: mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, false, log.DebugLevel, false),
		},
		{
			args:        []string{"plan", doubleDashed(run.ConfigFlagName)},
			expectedErr: argMissingValueError(run.ConfigFlagName),
		},

		{
			args:        []string{"plan", doubleDashed(global.WorkingDirFlagName)},
			expectedErr: argMissingValueError(global.WorkingDirFlagName),
		},

		{
			args:        []string{"plan", "--foo", "bar", doubleDashed(run.ConfigFlagName)},
			expectedErr: argMissingValueError(run.ConfigFlagName),
		},
		{
			args:            []string{"plan", doubleDashed(run.InputsDebugFlagName)},
			expectedOptions: mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, false, defaultLogLevel, true),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()

			l := log.New(
				log.WithOutput(os.Stderr),
				log.WithLevel(defaultLogLevel),
				log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
			)

			actualOptions, actualErr := runAppTest(l, tc.args, opts)

			if tc.expectedErr != nil {
				assert.EqualError(t, actualErr, tc.expectedErr.Error())
			} else {
				require.NoError(t, actualErr)
				assertOptionsEqual(t, *tc.expectedOptions, *actualOptions, "For args %v", tc.args)
			}
		})
	}
}

// We can't do a direct comparison between TerragruntOptions objects because we can't compare Logger or RunTerragrunt
// instances. Therefore, we have to manually check everything else.
func assertOptionsEqual(t *testing.T, expected options.TerragruntOptions, actual options.TerragruntOptions, msgAndArgs ...any) {
	t.Helper()

	// Normalize path separators for cross-platform compatibility
	expectedConfigPath := filepath.ToSlash(expected.TerragruntConfigPath)
	actualConfigPath := filepath.ToSlash(actual.TerragruntConfigPath)
	expectedWorkingDir := filepath.ToSlash(expected.WorkingDir)
	actualWorkingDir := filepath.ToSlash(actual.WorkingDir)

	assert.Equal(t, expectedConfigPath, actualConfigPath, msgAndArgs...)
	assert.Equal(t, expected.NonInteractive, actual.NonInteractive, msgAndArgs...)
	assert.Equal(t, expected.IncludeExternalDependencies, actual.IncludeExternalDependencies, msgAndArgs...)
	assert.Equal(t, expected.TerraformCliArgs, actual.TerraformCliArgs, msgAndArgs...)
	assert.Equal(t, expectedWorkingDir, actualWorkingDir, msgAndArgs...)
	assert.Equal(t, expected.Source, actual.Source, msgAndArgs...)
	assert.Equal(t, expected.IgnoreDependencyErrors, actual.IgnoreDependencyErrors, msgAndArgs...)
	assert.Equal(t, expected.IAMRoleOptions, actual.IAMRoleOptions, msgAndArgs...)
	assert.Equal(t, expected.OriginalIAMRoleOptions, actual.OriginalIAMRoleOptions, msgAndArgs...)
	assert.Equal(t, expected.Debug, actual.Debug, msgAndArgs...)
	assert.Equal(t, expected.SourceMap, actual.SourceMap, msgAndArgs...)
}

func mockOptions(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, includeExternalDependencies bool, _ log.Level, debug bool) *options.TerragruntOptions {
	t.Helper()

	// Normalize path separators for cross-platform compatibility
	terragruntConfigPath = filepath.ToSlash(terragruntConfigPath)
	workingDir = filepath.ToSlash(workingDir)

	opts, err := options.NewTerragruntOptionsForTest(terragruntConfigPath)
	if err != nil {
		t.Fatalf("error: %v\n", errors.New(err))
	}

	opts.WorkingDir = workingDir
	opts.TerraformCliArgs = terraformCliArgs
	opts.NonInteractive = nonInteractive
	opts.Source = terragruntSource
	opts.IgnoreDependencyErrors = ignoreDependencyErrors
	opts.IncludeExternalDependencies = includeExternalDependencies
	opts.Debug = debug

	return opts
}

func mockOptionsWithIamRole(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, iamRole string) *options.TerragruntOptions {
	t.Helper()

	opts := mockOptions(t, terragruntConfigPath, workingDir, terraformCliArgs, nonInteractive, terragruntSource, ignoreDependencyErrors, false, defaultLogLevel, false)
	opts.OriginalIAMRoleOptions.RoleARN = iamRole
	opts.IAMRoleOptions.RoleARN = iamRole

	return opts
}

func mockOptionsWithIamAssumeRoleDuration(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, iamAssumeRoleDuration int64) *options.TerragruntOptions {
	t.Helper()

	opts := mockOptions(t, terragruntConfigPath, workingDir, terraformCliArgs, nonInteractive, terragruntSource, ignoreDependencyErrors, false, defaultLogLevel, false)
	opts.OriginalIAMRoleOptions.AssumeRoleDuration = iamAssumeRoleDuration
	opts.IAMRoleOptions.AssumeRoleDuration = iamAssumeRoleDuration

	return opts
}

func mockOptionsWithIamAssumeRoleSessionName(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, iamAssumeRoleSessionName string) *options.TerragruntOptions {
	t.Helper()

	opts := mockOptions(t, terragruntConfigPath, workingDir, terraformCliArgs, nonInteractive, terragruntSource, ignoreDependencyErrors, false, defaultLogLevel, false)
	opts.OriginalIAMRoleOptions.AssumeRoleSessionName = iamAssumeRoleSessionName
	opts.IAMRoleOptions.AssumeRoleSessionName = iamAssumeRoleSessionName

	return opts
}

func mockOptionsWithIamWebIdentityToken(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, webIdentityToken string) *options.TerragruntOptions {
	t.Helper()

	opts := mockOptions(t, terragruntConfigPath, workingDir, terraformCliArgs, nonInteractive, terragruntSource, ignoreDependencyErrors, false, defaultLogLevel, false)
	opts.OriginalIAMRoleOptions.WebIdentityToken = webIdentityToken
	opts.IAMRoleOptions.WebIdentityToken = webIdentityToken

	return opts
}

func mockOptionsWithSourceMap(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, sourceMap map[string]string) *options.TerragruntOptions {
	t.Helper()

	opts := mockOptions(t, terragruntConfigPath, workingDir, terraformCliArgs, false, "", false, false, defaultLogLevel, false)
	opts.SourceMap = sourceMap

	return opts
}

func TestFilterTerragruntArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args     []string
		expected []string
	}{
		{[]string{}, []string{}},
		{[]string{"plan", "--bar"}, []string{"plan", "-bar"}},
		{[]string{"plan", doubleDashed(run.ConfigFlagName), "/some/path/" + config.DefaultTerragruntConfigPath}, []string{"plan"}},
		{[]string{"plan", doubleDashed(global.NonInteractiveFlagName)}, []string{"plan"}},
		{[]string{"plan", doubleDashed(run.InputsDebugFlagName)}, []string{"plan"}},
		{[]string{"plan", doubleDashed(global.NonInteractiveFlagName), "-bar", doubleDashed(global.WorkingDirFlagName), "/some/path", "--baz", doubleDashed(run.ConfigFlagName), "/some/path/" + config.DefaultTerragruntConfigPath}, []string{"plan", "-bar", "-baz"}},
		{[]string{"run", "--all", "apply", "plan", "bar"}, []string{tf.CommandNameApply, "plan", "bar"}},
		{[]string{"run", "--all", "destroy", "--", "plan", "-foo", "--bar"}, []string{tf.CommandNameDestroy, "plan", "-foo", "-bar"}},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()
			l := log.New(
				log.WithOutput(os.Stderr),
				log.WithLevel(defaultLogLevel),
				log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
			)
			actualOptions, err := runAppTest(l, tc.args, opts)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, []string(actualOptions.TerraformCliArgs), "For args %v", tc.args)
		})
	}
}

func TestParseMultiStringArg(t *testing.T) {
	t.Parallel()

	flagName := doubleDashed(run.UnitsThatIncludeFlagName)

	testCases := []struct {
		expectedErr  error
		args         []string
		defaultValue []string
		expectedVals []string
	}{
		{
			args:         []string{"run", "--all", "apply", flagName, "bar"},
			defaultValue: []string{"default_bar"},
			expectedVals: []string{"bar"},
		},
		{
			args:         []string{"run", "--all", "apply", "--", "--test", "bar"},
			defaultValue: []string{"default_bar"},
			expectedVals: []string{"default_bar"},
		},
		{
			args:         []string{"run", "--all", "plan", flagName, "bar1", flagName, "bar2", "--", "--test", "value"},
			defaultValue: []string{"default_bar"},
			expectedVals: []string{"bar1", "bar2"},
		},
		{
			args:         []string{"run", "--all", "plan", flagName, "bar1", flagName, "--", "--test", "value"},
			defaultValue: []string{"default_bar"},
			expectedErr:  argMissingValueError(run.UnitsThatIncludeFlagName),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()
			opts.ModulesThatInclude = tc.defaultValue
			l := log.New(
				log.WithOutput(os.Stderr),
				log.WithLevel(defaultLogLevel),
				log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
			)
			actualOptions, actualErr := runAppTest(l, tc.args, opts)

			if tc.expectedErr != nil {
				assert.EqualError(t, actualErr, tc.expectedErr.Error())
			} else {
				require.NoError(t, actualErr)
				assert.Equal(t, tc.expectedVals, actualOptions.ModulesThatInclude, "For args %q", tc.args)
			}
		})
	}
}

func TestParseMutliStringKeyValueArg(t *testing.T) {
	t.Parallel()

	flagName := doubleDashed(awsproviderpatch.OverrideAttrFlagName)

	testCases := []struct {
		expectedErr  error
		defaultValue map[string]string
		expectedVals map[string]string
		args         []string
	}{
		{
			args: []string{awsproviderpatch.CommandName},
		},
		{
			args:         []string{awsproviderpatch.CommandName},
			defaultValue: map[string]string{"default": "value"},
			expectedVals: map[string]string{"default": "value"},
		},
		{
			args:         []string{awsproviderpatch.CommandName, "--other", "arg"},
			defaultValue: map[string]string{"default": "value"},
			expectedVals: map[string]string{"default": "value"},
		},
		{
			args:         []string{awsproviderpatch.CommandName, flagName, "key=value"},
			defaultValue: map[string]string{"default": "value"},
			expectedVals: map[string]string{"key": "value"},
		},
		{
			args:         []string{awsproviderpatch.CommandName, flagName, "key1=value1", flagName, "key2=value2", flagName, "key3=value3"},
			defaultValue: map[string]string{"default": "value"},
			expectedVals: map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
		},
		{
			args:         []string{awsproviderpatch.CommandName, flagName, "invalidvalue"},
			defaultValue: map[string]string{"default": "value"},
			expectedErr:  clipkg.NewInvalidKeyValueError(clipkg.MapFlagKeyValSep, "invalidvalue"),
		},
	}

	for _, tc := range testCases {
		opts := options.NewTerragruntOptions()
		opts.AwsProviderPatchOverrides = tc.defaultValue
		l := log.New(
			log.WithOutput(os.Stderr),
			log.WithLevel(defaultLogLevel),
			log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
		)
		actualOptions, actualErr := runAppTest(l, tc.args, opts)

		if tc.expectedErr != nil {
			assert.ErrorContains(t, actualErr, tc.expectedErr.Error())
		} else {
			require.NoError(t, actualErr)
			assert.Equal(t, tc.expectedVals, actualOptions.AwsProviderPatchOverrides, "For args %v", tc.args)
		}
	}
}

func TestTerragruntVersion(t *testing.T) {
	t.Parallel()

	version := "v1.2.3"

	testCases := []struct {
		args []string
	}{
		{[]string{"terragrunt", "--version"}},
		{[]string{"terragrunt", "-version"}},
		{[]string{"terragrunt", "-v"}},
	}

	for _, tc := range testCases {
		output := &bytes.Buffer{}
		opts := options.NewTerragruntOptionsWithWriters(output, os.Stderr)
		app := cli.NewApp(logger.CreateLogger(), opts)
		app.Version = version

		err := app.Run(tc.args)
		require.NoError(t, err, tc)

		assert.Contains(t, output.String(), version)
	}
}

func TestTerragruntHelp(t *testing.T) {
	t.Parallel()

	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	opts := options.NewTerragruntOptions()
	app := cli.NewApp(logger.CreateLogger(), opts)

	testCases := []struct {
		expected    string
		notExpected string
		args        []string
	}{
		{
			args:        []string{"terragrunt", "--help"},
			expected:    app.UsageText,
			notExpected: terragruntPrefix.FlagName(awsproviderpatch.OverrideAttrFlagName),
		},
		{
			args:        []string{"terragrunt", "-help"},
			expected:    app.UsageText,
			notExpected: terragruntPrefix.FlagName(awsproviderpatch.OverrideAttrFlagName),
		},
		{
			args:        []string{"terragrunt", "-h"},
			expected:    app.UsageText,
			notExpected: terragruntPrefix.FlagName(awsproviderpatch.OverrideAttrFlagName),
		},
		{
			args:        []string{"terragrunt", awsproviderpatch.CommandName, "-h"},
			expected:    run.ConfigFlagName,
			notExpected: hcl.CommandName + " " + hclformat.CommandName,
		},
		{
			args:     []string{"terragrunt", run.CommandName, "--help"},
			expected: run.CommandName,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			output := &bytes.Buffer{}
			opts := options.NewTerragruntOptionsWithWriters(output, os.Stderr)
			app := cli.NewApp(logger.CreateLogger(), opts)
			err := app.Run(tc.args)
			require.NoError(t, err, tc)

			assert.Contains(t, output.String(), tc.expected)

			if tc.notExpected != "" {
				assert.NotContains(t, output.String(), tc.notExpected)
			}
		})
	}
}

func TestTerraformHelp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expected string
		args     []string
	}{
		{args: []string{"terragrunt", tf.CommandNamePlan, "--help"}, expected: "(?s)Usage: terragrunt \\[global options\\] plan.*-detailed-exitcode"},
		{args: []string{"terragrunt", tf.CommandNameApply, "-help"}, expected: "(?s)Usage: terragrunt \\[global options\\] apply.*-destroy"},
		{args: []string{"terragrunt", tf.CommandNameApply, "-h"}, expected: "(?s)Usage: terragrunt \\[global options\\] apply.*-destroy"},
	}

	for _, tc := range testCases {
		output := &bytes.Buffer{}
		opts := options.NewTerragruntOptionsWithWriters(output, os.Stderr)
		app := cli.NewApp(logger.CreateLogger(), opts)
		err := app.Run(tc.args)
		require.NoError(t, err)

		assert.Regexp(t, tc.expected, output.String())
	}
}

func TestTerraformHelp_wrongHelpFlag(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}

	opts := options.NewTerragruntOptionsWithWriters(output, os.Stderr)
	app := cli.NewApp(logger.CreateLogger(), opts)

	err := app.Run([]string{"terragrunt", "plan", "help"})
	require.Error(t, err)
}

func setCommandAction(action clipkg.ActionFunc, cmds ...*clipkg.Command) {
	for _, cmd := range cmds {
		cmd.Action = action
		setCommandAction(action, cmd.Subcommands...)
	}
}

func runAppTest(l log.Logger, args []string, opts *options.TerragruntOptions) (*options.TerragruntOptions, error) {
	emptyAction := func(ctx *clipkg.Context) error { return nil }

	terragruntCommands := commands.New(l, opts)
	setCommandAction(emptyAction, terragruntCommands...)

	app := clipkg.NewApp()
	app.Writer = &bytes.Buffer{}
	app.ErrWriter = &bytes.Buffer{}

	app.Flags = append(global.NewFlags(l, opts, nil), run.NewFlags(l, opts, nil)...)
	app.Commands = terragruntCommands.WrapAction(commands.WrapWithTelemetry(l, opts))
	app.OsExiter = cli.OSExiter
	app.Action = func(ctx *clipkg.Context) error {
		opts.TerraformCliArgs = append(opts.TerraformCliArgs, ctx.Args()...)
		return nil
	}
	app.ExitErrHandler = cli.ExitErrHandler

	err := app.Run(append([]string{"--"}, args...))

	return opts, err
}

func doubleDashed(name string) string {
	return "--" + name
}

type argMissingValueError string

func (err argMissingValueError) Error() string {
	return "flag needs an argument: -" + string(err)
}

func TestAutocomplete(t *testing.T) { //nolint:paralleltest
	testCases := []struct {
		compLine          string
		expectedCompletes []string
	}{
		{
			"",
			[]string{"hcl", "render", "run"},
		},
		{
			"--versio",
			[]string{"--version"},
		},
		{
			"render -",
			[]string{"--out", "--with-metadata"},
		},
		{
			"run pla",
			[]string{"plan"},
		},
	}

	for _, tc := range testCases {
		t.Setenv("COMP_LINE", "terragrunt "+tc.compLine)

		output := &bytes.Buffer{}
		opts := options.NewTerragruntOptionsWithWriters(output, os.Stderr)
		app := cli.NewApp(logger.CreateLogger(), opts)

		app.Commands = app.Commands.FilterByNames([]string{"hcl", "render", "run"})

		err := app.Run([]string{"terragrunt"})
		require.NoError(t, err)

		for _, expectedComplete := range tc.expectedCompletes {
			assert.Contains(t, output.String(), expectedComplete)
		}
	}
}
