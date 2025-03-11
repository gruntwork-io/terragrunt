package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/cli/commands"
	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	outputmodulegroups "github.com/gruntwork-io/terragrunt/cli/commands/output-module-groups"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/config"
	clipkg "github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var defaultLogLevel = log.DebugLevel

func TestParseTerragruntOptionsFromArgs(t *testing.T) {
	t.Parallel()

	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	workingDir = filepath.ToSlash(workingDir)

	testCases := []struct {
		args            []string
		expectedOptions *options.TerragruntOptions
		expectedErr     error
	}{
		{
			[]string{"plan"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"plan", "bar"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan", "bar"}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--foo", "--bar"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"-foo", "-bar"}, false, "", false, false, defaultLogLevel, false),
			clipkg.UndefinedFlagError("foo"),
		},

		{
			[]string{"--foo", "apply", "--bar"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"apply", "-foo", "-bar"}, false, "", false, false, defaultLogLevel, false),
			clipkg.UndefinedFlagError("foo"),
		},

		{
			[]string{doubleDashed(global.NonInteractiveFlagName)},
			mockOptions(t, "", "", []string{}, true, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"apply", doubleDashed(terragruntPrefix.FlagName(run.DeprecatedIncludeExternalDependenciesFlagName))},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"apply"}, false, "", false, true, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"plan", doubleDashed(run.ConfigFlagName), "/some/path/" + config.DefaultTerragruntConfigPath},
			mockOptions(t, "/some/path/"+config.DefaultTerragruntConfigPath, workingDir, []string{"plan"}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"plan", doubleDashed(global.WorkingDirFlagName), "/some/path"},
			mockOptions(t, util.JoinPath("/some/path", config.DefaultTerragruntConfigPath), "/some/path", []string{"plan"}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"plan", doubleDashed(run.SourceFlagName), "/some/path"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "/some/path", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"plan", doubleDashed(run.SourceMapFlagName), "git::git@github.com:one/gw-terraform-aws-vpc.git=git::git@github.com:two/test.git?ref=FEATURE"},
			mockOptionsWithSourceMap(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, map[string]string{"git::git@github.com:one/gw-terraform-aws-vpc.git": "git::git@github.com:two/test.git?ref=FEATURE"}),
			nil,
		},

		{
			[]string{"plan", doubleDashed(run.QueueIgnoreErrorsFlagName)},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", true, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"plan", doubleDashed(terragruntPrefix.FlagName(run.DeprecatedIgnoreExternalDependenciesFlagName))},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"plan", doubleDashed(terragruntPrefix.FlagName(run.DeprecatedIAMRoleFlagName)), "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"},
			mockOptionsWithIamRole(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"),
			nil,
		},

		{
			[]string{"plan", doubleDashed(run.IAMAssumeRoleDurationFlagName), "36000"},
			mockOptionsWithIamAssumeRoleDuration(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, 36000),
			nil,
		},

		{
			[]string{"plan", doubleDashed(run.IAMAssumeRoleSessionNameFlagName), "terragrunt-iam-role-session-name"},
			mockOptionsWithIamAssumeRoleSessionName(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, "terragrunt-iam-role-session-name"),
			nil,
		},

		{
			[]string{"plan", doubleDashed(terragruntPrefix.FlagName(run.DeprecatedIAMWebIdentityTokenFlagName)), "web-identity-token"},
			mockOptionsWithIamWebIdentityToken(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, "web-identity-token"),
			nil,
		},

		{
			[]string{"plan", doubleDashed(run.ConfigFlagName), "/some/path/" + config.DefaultTerragruntConfigPath, "--terragrunt-non-interactive"},
			mockOptions(t, "/some/path/"+config.DefaultTerragruntConfigPath, workingDir, []string{"plan"}, true, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"plan", doubleDashed(run.ConfigFlagName), "/some/path/" + config.DefaultTerragruntConfigPath, "bar", doubleDashed(global.NonInteractiveFlagName), "--baz", doubleDashed(global.WorkingDirFlagName), "/some/path", doubleDashed(run.SourceFlagName), "github.com/foo/bar//baz?ref=1.0.3"},
			mockOptions(t, "/some/path/"+config.DefaultTerragruntConfigPath, "/some/path", []string{"plan", "bar", "-baz"}, true, "github.com/foo/bar//baz?ref=1.0.3", false, false, defaultLogLevel, false),
			nil,
		},

		// Adding the --terragrunt-log-level flag should result in DebugLevel configured
		{
			[]string{"plan", doubleDashed(global.LogLevelFlagName), "debug"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, false, log.DebugLevel, false),
			nil,
		},
		{
			[]string{"plan", doubleDashed(run.ConfigFlagName)},
			nil,
			argMissingValueError(run.ConfigFlagName),
		},

		{
			[]string{"plan", doubleDashed(global.WorkingDirFlagName)},
			nil,
			argMissingValueError(global.WorkingDirFlagName),
		},

		{
			[]string{"plan", "--foo", "bar", doubleDashed(run.ConfigFlagName)},
			nil,
			argMissingValueError(run.ConfigFlagName),
		},
		{
			[]string{"plan", doubleDashed(run.InputsDebugFlagName)},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"plan"}, false, "", false, false, defaultLogLevel, true),
			nil,
		},
		{
			[]string{outputmodulegroups.CommandName, outputmodulegroups.SubCommandApply},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{outputmodulegroups.SubCommandApply}, false, "", false, false, defaultLogLevel, false),
			nil,
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()
			actualOptions, actualErr := runAppTest(testCase.args, opts)

			if testCase.expectedErr != nil {
				assert.EqualError(t, actualErr, testCase.expectedErr.Error())
			} else {
				require.NoError(t, actualErr)
				assertOptionsEqual(t, *testCase.expectedOptions, *actualOptions, "For args %v", testCase.args)
			}
		})
	}
}

// We can't do a direct comparison between TerragruntOptions objects because we can't compare Logger or RunTerragrunt
// instances. Therefore, we have to manually check everything else.
func assertOptionsEqual(t *testing.T, expected options.TerragruntOptions, actual options.TerragruntOptions, msgAndArgs ...interface{}) {
	t.Helper()

	assert.NotNil(t, expected.Logger, msgAndArgs...)
	assert.NotNil(t, actual.Logger, msgAndArgs...)

	assert.Equal(t, expected.TerragruntConfigPath, actual.TerragruntConfigPath, msgAndArgs...)
	assert.Equal(t, expected.NonInteractive, actual.NonInteractive, msgAndArgs...)
	assert.Equal(t, expected.IncludeExternalDependencies, actual.IncludeExternalDependencies, msgAndArgs...)
	assert.Equal(t, expected.TerraformCliArgs, actual.TerraformCliArgs, msgAndArgs...)
	assert.Equal(t, expected.WorkingDir, actual.WorkingDir, msgAndArgs...)
	assert.Equal(t, expected.Source, actual.Source, msgAndArgs...)
	assert.Equal(t, expected.IgnoreDependencyErrors, actual.IgnoreDependencyErrors, msgAndArgs...)
	assert.Equal(t, expected.IAMRoleOptions, actual.IAMRoleOptions, msgAndArgs...)
	assert.Equal(t, expected.OriginalIAMRoleOptions, actual.OriginalIAMRoleOptions, msgAndArgs...)
	assert.Equal(t, expected.Debug, actual.Debug, msgAndArgs...)
	assert.Equal(t, expected.SourceMap, actual.SourceMap, msgAndArgs...)
}

func mockOptions(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, includeExternalDependencies bool, logLevel log.Level, debug bool) *options.TerragruntOptions {
	t.Helper()

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
	opts.Logger.SetOptions(log.WithLevel(logLevel))
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
		{[]string{commands.CommandNameApplyAll, "plan", "bar"}, []string{tf.CommandNameApply, "plan", "bar"}},
		{[]string{commands.CommandNameDestroyAll, "plan", "-foo", "--bar"}, []string{tf.CommandNameDestroy, "plan", "-foo", "-bar"}},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()
			opts := options.NewTerragruntOptions()
			actualOptions, err := runAppTest(testCase.args, opts)
			require.NoError(t, err)
			assert.Equal(t, testCase.expected, []string(actualOptions.TerraformCliArgs), "For args %v", testCase.args)
		})
	}
}

func TestParseMultiStringArg(t *testing.T) {
	t.Parallel()

	flagName := doubleDashed(run.UnitsThatIncludeFlagName)

	testCases := []struct {
		args         []string
		defaultValue []string
		expectedVals []string
		expectedErr  error
	}{
		{[]string{commands.CommandNameApplyAll, flagName, "bar"}, []string{"default_bar"}, []string{"bar"}, nil},
		{[]string{commands.CommandNameApplyAll, "--test", "bar"}, []string{"default_bar"}, []string{"default_bar"}, nil},
		{[]string{commands.CommandNamePlanAll, "--test", flagName, "bar1", flagName, "bar2"}, []string{"default_bar"}, []string{"bar1", "bar2"}, nil},
		{[]string{commands.CommandNamePlanAll, "--test", "value", flagName, "bar1", flagName}, []string{"default_bar"}, nil, argMissingValueError(run.UnitsThatIncludeFlagName)},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()
			opts.ModulesThatInclude = testCase.defaultValue
			actualOptions, actualErr := runAppTest(testCase.args, opts)

			if testCase.expectedErr != nil {
				assert.EqualError(t, actualErr, testCase.expectedErr.Error())
			} else {
				require.NoError(t, actualErr)
				assert.Equal(t, testCase.expectedVals, actualOptions.ModulesThatInclude, "For args %q", testCase.args)
			}
		})
	}
}

func TestParseMutliStringKeyValueArg(t *testing.T) {
	t.Parallel()

	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	flagName := doubleDashed(terragruntPrefix.FlagName(awsproviderpatch.OverrideAttrFlagName))

	testCases := []struct {
		args         []string
		defaultValue map[string]string
		expectedVals map[string]string
		expectedErr  error
	}{
		{[]string{awsproviderpatch.CommandName}, nil, nil, nil},
		{[]string{awsproviderpatch.CommandName}, map[string]string{"default": "value"}, map[string]string{"default": "value"}, nil},
		{[]string{awsproviderpatch.CommandName, "--other", "arg"}, map[string]string{"default": "value"}, map[string]string{"default": "value"}, nil},
		{[]string{awsproviderpatch.CommandName, flagName, "key=value"}, map[string]string{"default": "value"}, map[string]string{"key": "value"}, nil},
		{[]string{awsproviderpatch.CommandName, flagName, "key1=value1", flagName, "key2=value2", flagName, "key3=value3"}, map[string]string{"default": "value"}, map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"}, nil},
		{[]string{awsproviderpatch.CommandName, flagName, "invalidvalue"}, map[string]string{"default": "value"}, nil, clipkg.NewInvalidKeyValueError(clipkg.MapFlagKeyValSep, "invalidvalue")},
	}

	for _, testCase := range testCases {
		opts := options.NewTerragruntOptions()
		opts.AwsProviderPatchOverrides = testCase.defaultValue
		actualOptions, actualErr := runAppTest(testCase.args, opts)

		if testCase.expectedErr != nil {
			assert.ErrorContains(t, actualErr, testCase.expectedErr.Error())
		} else {
			require.NoError(t, actualErr)
			assert.Equal(t, testCase.expectedVals, actualOptions.AwsProviderPatchOverrides, "For args %v", testCase.args)
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

	for _, testCase := range testCases {
		output := &bytes.Buffer{}
		opts := options.NewTerragruntOptionsWithWriters(output, os.Stderr)
		app := cli.NewApp(opts)
		app.Version = version

		err := app.Run(testCase.args)
		require.NoError(t, err, testCase)

		assert.Contains(t, output.String(), version)
	}
}

func TestTerragruntHelp(t *testing.T) {
	t.Parallel()

	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	opts := options.NewTerragruntOptions()
	app := cli.NewApp(opts)

	testCases := []struct {
		args        []string
		expected    string
		notExpected string
	}{
		{[]string{"terragrunt", "--help"}, app.UsageText, terragruntPrefix.FlagName(awsproviderpatch.OverrideAttrFlagName)},
		{[]string{"terragrunt", "-help"}, app.UsageText, terragruntPrefix.FlagName(awsproviderpatch.OverrideAttrFlagName)},
		{[]string{"terragrunt", "-h"}, app.UsageText, terragruntPrefix.FlagName(awsproviderpatch.OverrideAttrFlagName)},
		{[]string{"terragrunt", awsproviderpatch.CommandName, "-h"}, run.ConfigFlagName, hclfmt.CommandName},
		{[]string{"terragrunt", commands.CommandNamePlanAll, "--help"}, runall.CommandName, ""},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			output := &bytes.Buffer{}
			opts := options.NewTerragruntOptionsWithWriters(output, os.Stderr)
			app := cli.NewApp(opts)
			err := app.Run(testCase.args)
			require.NoError(t, err, testCase)

			assert.Contains(t, output.String(), testCase.expected)
			if testCase.notExpected != "" {
				assert.NotContains(t, output.String(), testCase.notExpected)
			}
		})
	}
}

func TestTerraformHelp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args     []string
		expected string
	}{
		{[]string{"terragrunt", tf.CommandNamePlan, "--help"}, "(?s)Usage: terragrunt \\[global options\\] plan.*-detailed-exitcode"},
		{[]string{"terragrunt", tf.CommandNameApply, "-help"}, "(?s)Usage: terragrunt \\[global options\\] apply.*-destroy"},
		{[]string{"terragrunt", tf.CommandNameApply, "-h"}, "(?s)Usage: terragrunt \\[global options\\] apply.*-destroy"},
	}

	for _, testCase := range testCases {
		output := &bytes.Buffer{}
		opts := options.NewTerragruntOptionsWithWriters(output, os.Stderr)
		app := cli.NewApp(opts)
		err := app.Run(testCase.args)
		require.NoError(t, err)

		assert.Regexp(t, testCase.expected, output.String())
	}
}

func TestTerraformHelp_wrongHelpFlag(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}

	opts := options.NewTerragruntOptionsWithWriters(output, os.Stderr)
	app := cli.NewApp(opts)

	err := app.Run([]string{"terragrunt", "plan", "help"})
	require.Error(t, err)
}

func setCommandAction(action clipkg.ActionFunc, cmds ...*clipkg.Command) {
	for _, cmd := range cmds {
		cmd.Action = action
		setCommandAction(action, cmd.Subcommands...)
	}
}

func runAppTest(args []string, opts *options.TerragruntOptions) (*options.TerragruntOptions, error) {
	emptyAction := func(ctx *clipkg.Context) error { return nil }

	terragruntCommands := commands.New(opts)
	setCommandAction(emptyAction, terragruntCommands...)

	app := clipkg.NewApp()
	app.Writer = &bytes.Buffer{}
	app.ErrWriter = &bytes.Buffer{}
	app.Flags = append(global.NewFlags(opts, nil), run.NewFlags(opts, nil)...)
	app.Commands = append(
		commands.NewDeprecatedCommands(opts),
		terragruntCommands...).WrapAction(cli.WrapWithTelemetry(opts))
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
			[]string{"graph-dependencies", "hclfmt", "output-module-groups", "render-json", "run-all", "terragrunt-info", "validate-inputs"},
		},
		{
			"--versio",
			[]string{"--version"},
		},
		{
			"render-json -",
			[]string{"--out", "--with-metadata"},
		},
		{
			"run-all ren",
			[]string{"render-json"},
		},
	}

	for _, testCase := range testCases {
		t.Setenv("COMP_LINE", "terragrunt "+testCase.compLine)

		output := &bytes.Buffer{}
		opts := options.NewTerragruntOptionsWithWriters(output, os.Stderr)
		app := cli.NewApp(opts)

		app.Commands = app.Commands.FilterByNames([]string{"graph-dependencies", "hclfmt", "output-module-groups", "render-json", "run-all", "terragrunt-info", "validate-inputs"})

		err := app.Run([]string{"terragrunt"})
		require.NoError(t, err)

		for _, expectedComplete := range testCase.expectedCompletes {
			assert.Contains(t, output.String(), expectedComplete)
		}
	}
}
