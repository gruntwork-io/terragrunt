package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/cli/commands"
	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	terraformcmd "github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var defaultLogLevel = util.GetDefaultLogLevel()

func TestParseTerragruntOptionsFromArgs(t *testing.T) {
	t.Parallel()

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
			[]string{},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"foo", "bar"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"foo", "bar"}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--foo", "--bar"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"-foo", "-bar"}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--foo", "apply", "--bar"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"-foo", "apply", "-bar"}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntNonInteractiveFlagName)},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, true, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntIncludeExternalDependenciesFlagName)},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, true, defaultLogLevel, false),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntConfigFlagName), fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath)},
			mockOptions(t, fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntWorkingDirFlagName), "/some/path"},
			mockOptions(t, util.JoinPath("/some/path", config.DefaultTerragruntConfigPath), "/some/path", []string{}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntSourceFlagName), "/some/path"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "/some/path", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntSourceMapFlagName), "git::git@github.com:one/gw-terraform-aws-vpc.git=git::git@github.com:two/test.git?ref=FEATURE"},
			mockOptionsWithSourceMap(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, map[string]string{"git::git@github.com:one/gw-terraform-aws-vpc.git": "git::git@github.com:two/test.git?ref=FEATURE"}),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntIgnoreDependencyErrorsFlagName)},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", true, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntIgnoreExternalDependenciesFlagName)},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntIAMRoleFlagName), "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"},
			mockOptionsWithIamRole(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntIAMAssumeRoleDurationFlagName), "36000"},
			mockOptionsWithIamAssumeRoleDuration(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, 36000),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntIAMAssumeRoleSessionNameFlagName), "terragrunt-iam-role-session-name"},
			mockOptionsWithIamAssumeRoleSessionName(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, "terragrunt-iam-role-session-name"),
			nil,
		},

		{
			[]string{doubleDashed(commands.TerragruntConfigFlagName), fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath), "--terragrunt-non-interactive"},
			mockOptions(t, fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath), workingDir, []string{}, true, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--foo", doubleDashed(commands.TerragruntConfigFlagName), fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath), "bar", doubleDashed(commands.TerragruntNonInteractiveFlagName), "--baz", doubleDashed(commands.TerragruntWorkingDirFlagName), "/some/path", doubleDashed(commands.TerragruntSourceFlagName), "github.com/foo/bar//baz?ref=1.0.3"},
			mockOptions(t, fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath), "/some/path", []string{"-foo", "bar", "-baz"}, true, "github.com/foo/bar//baz?ref=1.0.3", false, false, defaultLogLevel, false),
			nil,
		},

		// Adding the --terragrunt-log-level flag should result in DebugLevel configured
		{
			[]string{doubleDashed(commands.TerragruntLogLevelFlagName), "debug"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, false, logrus.DebugLevel, false),
			nil,
		},
		{
			[]string{doubleDashed(commands.TerragruntConfigFlagName)},
			nil,
			argMissingValueError(commands.TerragruntConfigFlagName),
		},

		{
			[]string{doubleDashed(commands.TerragruntWorkingDirFlagName)},
			nil,
			argMissingValueError(commands.TerragruntWorkingDirFlagName),
		},

		{
			[]string{"--foo", "bar", doubleDashed(commands.TerragruntConfigFlagName)},
			nil,
			argMissingValueError(commands.TerragruntConfigFlagName),
		},
		{
			[]string{doubleDashed(commands.TerragruntDebugFlagName)},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, false, defaultLogLevel, true),
			nil,
		},
	}

	for _, testCase := range testCases {
		opts := options.NewTerragruntOptions()
		actualOptions, actualErr := runAppTest(testCase.args, opts)

		if testCase.expectedErr != nil {
			assert.EqualError(t, actualErr, testCase.expectedErr.Error())
		} else {
			assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
			assertOptionsEqual(t, *testCase.expectedOptions, *actualOptions, "For args %v", testCase.args)
		}
	}
}

// We can't do a direct comparison between TerragruntOptions objects because we can't compare Logger or RunTerragrunt
// instances. Therefore, we have to manually check everything else.
func assertOptionsEqual(t *testing.T, expected options.TerragruntOptions, actual options.TerragruntOptions, msgAndArgs ...interface{}) {
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

func mockOptions(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, includeExternalDependencies bool, logLevel logrus.Level, debug bool) *options.TerragruntOptions {
	opts, err := options.NewTerragruntOptionsForTest(terragruntConfigPath)
	if err != nil {
		t.Fatalf("error: %v\n", errors.WithStackTrace(err))
	}

	opts.WorkingDir = workingDir
	opts.TerraformCliArgs = terraformCliArgs
	opts.NonInteractive = nonInteractive
	opts.Source = terragruntSource
	opts.IgnoreDependencyErrors = ignoreDependencyErrors
	opts.IncludeExternalDependencies = includeExternalDependencies
	opts.Logger.Level = logLevel
	opts.Debug = debug

	return opts
}

func mockOptionsWithIamRole(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, iamRole string) *options.TerragruntOptions {
	opts := mockOptions(t, terragruntConfigPath, workingDir, terraformCliArgs, nonInteractive, terragruntSource, ignoreDependencyErrors, false, defaultLogLevel, false)
	opts.OriginalIAMRoleOptions.RoleARN = iamRole
	opts.IAMRoleOptions.RoleARN = iamRole

	return opts
}

func mockOptionsWithIamAssumeRoleDuration(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, iamAssumeRoleDuration int64) *options.TerragruntOptions {
	opts := mockOptions(t, terragruntConfigPath, workingDir, terraformCliArgs, nonInteractive, terragruntSource, ignoreDependencyErrors, false, defaultLogLevel, false)
	opts.OriginalIAMRoleOptions.AssumeRoleDuration = iamAssumeRoleDuration
	opts.IAMRoleOptions.AssumeRoleDuration = iamAssumeRoleDuration

	return opts
}

func mockOptionsWithIamAssumeRoleSessionName(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, iamAssumeRoleSessionName string) *options.TerragruntOptions {
	opts := mockOptions(t, terragruntConfigPath, workingDir, terraformCliArgs, nonInteractive, terragruntSource, ignoreDependencyErrors, false, defaultLogLevel, false)
	opts.OriginalIAMRoleOptions.AssumeRoleSessionName = iamAssumeRoleSessionName
	opts.IAMRoleOptions.AssumeRoleSessionName = iamAssumeRoleSessionName

	return opts
}

func mockOptionsWithSourceMap(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, sourceMap map[string]string) *options.TerragruntOptions {
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
		{[]string{"foo", "--bar"}, []string{"foo", "-bar"}},
		{[]string{"foo", doubleDashed(commands.TerragruntConfigFlagName), fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath)}, []string{"foo"}},
		{[]string{"foo", doubleDashed(commands.TerragruntNonInteractiveFlagName)}, []string{"foo"}},
		{[]string{"foo", doubleDashed(commands.TerragruntDebugFlagName)}, []string{"foo"}},
		{[]string{"foo", doubleDashed(commands.TerragruntNonInteractiveFlagName), "-bar", doubleDashed(commands.TerragruntWorkingDirFlagName), "/some/path", "--baz", doubleDashed(commands.TerragruntConfigFlagName), fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath)}, []string{"foo", "-bar", "-baz"}},
		{[]string{CommandNameApplyAll, "foo", "bar"}, []string{terraform.CommandNameApply, "foo", "bar"}},
		{[]string{CommandNameDestroyAll, "foo", "-foo", "--bar"}, []string{terraform.CommandNameDestroy, "foo", "-foo", "-bar"}},
	}

	for _, testCase := range testCases {
		opts := options.NewTerragruntOptions()
		actualOptions, _ := runAppTest(testCase.args, opts)
		assert.Equal(t, testCase.expected, actualOptions.TerraformCliArgs, "For args %v", testCase.args)
	}
}

func TestParseMultiStringArg(t *testing.T) {
	t.Parallel()

	flagName := doubleDashed(commands.TerragruntModulesThatIncludeFlagName)

	testCases := []struct {
		args         []string
		defaultValue []string
		expectedVals []string
		expectedErr  error
	}{
		{[]string{CommandNameApplyAll, flagName, "bar"}, []string{"default_bar"}, []string{"bar"}, nil},
		{[]string{CommandNameApplyAll, "--test", "bar"}, []string{"default_bar"}, []string{"default_bar"}, nil},
		{[]string{CommandNamePlanAll, "--test", flagName, "bar1", flagName, "bar2"}, []string{"default_bar"}, []string{"bar1", "bar2"}, nil},
		{[]string{CommandNamePlanAll, "--test", "value", flagName, "bar1", flagName}, []string{"default_bar"}, nil, argMissingValueError(commands.TerragruntModulesThatIncludeFlagName)},
	}

	for _, testCase := range testCases {
		opts := options.NewTerragruntOptions()
		opts.ModulesThatInclude = testCase.defaultValue
		actualOptions, actualErr := runAppTest(testCase.args, opts)

		if testCase.expectedErr != nil {
			assert.EqualError(t, actualErr, testCase.expectedErr.Error())
		} else {
			assert.Nil(t, actualErr, "Unexpected error: %q", actualErr)
			assert.Equal(t, testCase.expectedVals, actualOptions.ModulesThatInclude, "For args %q", testCase.args)
		}
	}
}

func TestParseMutliStringKeyValueArg(t *testing.T) {
	t.Parallel()

	flagName := doubleDashed(awsproviderpatch.FlagNameTerragruntOverrideAttr)

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
		{[]string{awsproviderpatch.CommandName, flagName, "invalidvalue"}, map[string]string{"default": "value"}, nil, cli.NewInvalidKeyValueError(cli.MapFlagKeyValSep, "invalidvalue")},
	}

	for _, testCase := range testCases {
		opts := options.NewTerragruntOptions()
		opts.AwsProviderPatchOverrides = testCase.defaultValue
		actualOptions, actualErr := runAppTest(testCase.args, opts)

		if testCase.expectedErr != nil {
			assert.ErrorContains(t, actualErr, testCase.expectedErr.Error())
		} else {
			assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
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
		app := NewApp(output, os.Stderr)
		app.Version = version

		err := app.Run(testCase.args)
		require.NoError(t, err, testCase)

		assert.Contains(t, output.String(), version)
	}
}

func TestTerragruntHelp(t *testing.T) {
	t.Parallel()

	app := NewApp(os.Stdout, os.Stderr)

	testCases := []struct {
		args        []string
		expected    string
		notExpected string
	}{
		{[]string{"terragrunt", "--help"}, app.UsageText, awsproviderpatch.FlagNameTerragruntOverrideAttr},
		{[]string{"terragrunt", "-help"}, app.UsageText, awsproviderpatch.FlagNameTerragruntOverrideAttr},
		{[]string{"terragrunt", "-h"}, app.UsageText, awsproviderpatch.FlagNameTerragruntOverrideAttr},
		{[]string{"terragrunt", awsproviderpatch.CommandName, "-h"}, commands.TerragruntConfigFlagName, hclfmt.CommandName},
		{[]string{"terragrunt", CommandNamePlanAll, "--help"}, runall.CommandName, ""},
	}

	for _, testCase := range testCases {
		output := &bytes.Buffer{}
		app := NewApp(output, os.Stderr)
		err := app.Run(testCase.args)
		require.NoError(t, err, testCase)

		assert.Contains(t, output.String(), testCase.expected)
		if testCase.notExpected != "" {
			assert.NotContains(t, output.String(), testCase.notExpected)
		}
	}
}

func TestTerraformHelp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args     []string
		expected string
	}{
		{[]string{"terragrunt", terraform.CommandNamePlan, "--help"}, "Usage: terraform .* plan"},
		{[]string{"terragrunt", terraform.CommandNameApply, "-help"}, "Usage: terraform .* apply"},
		{[]string{"terragrunt", terraform.CommandNameApply, "-h"}, "Usage: terraform .* apply"},
	}

	for _, testCase := range testCases {
		output := &bytes.Buffer{}
		app := NewApp(output, os.Stderr)
		err := app.Run(testCase.args)
		require.NoError(t, err)

		expectedRegex, err := regexp.Compile(testCase.expected)
		require.NoError(t, err)

		assert.Regexp(t, expectedRegex, output.String())
	}
}

func TestTerraformHelp_wrongHelpFlag(t *testing.T) {
	t.Parallel()

	app := NewApp(os.Stdout, os.Stderr)

	output := &bytes.Buffer{}
	app.Writer = output

	err := app.Run([]string{"terragrunt", "plan", "help"})
	require.Error(t, err)
}

func runAppTest(args []string, opts *options.TerragruntOptions) (*options.TerragruntOptions, error) {
	testAction := func(cliCtx *cli.Context) error {
		return initialSetup(cliCtx, opts)
	}

	terragruntCommands := terragruntCommands(opts)
	for _, command := range terragruntCommands {
		command.Action = testAction
	}

	app := cli.NewApp()
	app.Writer = &bytes.Buffer{}
	app.ErrWriter = &bytes.Buffer{}
	app.Flags = append(
		commands.NewGlobalFlags(opts),
		commands.NewHelpVersionFlags(opts)...)
	app.Commands = append(
		deprecatedCommands(opts),
		terragruntCommands...)
	app.DefaultCommand = terraformcmd.NewCommand(opts)
	app.DefaultCommand.Action = testAction
	app.OsExiter = osExiter

	err := app.Run(append([]string{"--"}, args...))
	return opts, err
}

func doubleDashed(name string) string {
	return fmt.Sprintf("--%s", name)
}

type argMissingValueError string

func (err argMissingValueError) Error() string {
	return fmt.Sprintf("flag needs an argument: -%s", string(err))
}

func TestAutocomplete(t *testing.T) {
	defer os.Unsetenv("COMP_LINE")

	testCases := []struct {
		compLine          string
		expectedCompletes []string
	}{
		{
			"",
			[]string{"aws-provider-patch", "graph-dependencies", "hclfmt", "output-module-groups", "render-json", "run-all", "terragrunt-info", "validate-inputs"},
		},
		{
			"--versio",
			[]string{"--version"},
		},
		{
			"render-json -",
			[]string{"--terragrunt-json-out", "--with-metadata"},
		},
		{
			"run-all ren",
			[]string{"render-json"},
		},
	}

	for _, testCase := range testCases {
		os.Setenv("COMP_LINE", "terragrunt "+testCase.compLine)

		output := &bytes.Buffer{}
		app := NewApp(output, os.Stderr)
		app.Commands = app.Commands.Filter([]string{"aws-provider-patch", "graph-dependencies", "hclfmt", "output-module-groups", "render-json", "run-all", "terragrunt-info", "validate-inputs"})

		err := app.Run([]string{"terragrunt"})
		require.NoError(t, err)

		assert.Contains(t, output.String(), strings.Join(testCase.expectedCompletes, "\n"))
	}
}
