package cli

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
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
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"--foo", "--bar"}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--foo", "apply", "--bar"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"--foo", "apply", "--bar"}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--terragrunt-non-interactive"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, true, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--terragrunt-include-external-dependencies"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, true, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--terragrunt-config", fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath)},
			mockOptions(t, fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--terragrunt-working-dir", "/some/path"},
			mockOptions(t, util.JoinPath("/some/path", config.DefaultTerragruntConfigPath), "/some/path", []string{}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--terragrunt-source", "/some/path"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "/some/path", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--terragrunt-override-attr", "region=us-east-1"},
			mockOptionsWithOverrideAttrs(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, map[string]string{"region": "us-east-1"}),
			nil,
		},

		{
			[]string{"--terragrunt-ignore-dependency-errors"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", true, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--terragrunt-ignore-external-dependencies"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--terragrunt-iam-role", "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"},
			mockOptionsWithIamRole(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"),
			nil,
		},

		{
			[]string{"--terragrunt-iam-assume-role-duration", "36000"},
			mockOptionsWithIamAssumeRoleDuration(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, 36000),
			nil,
		},

		{
			[]string{"--terragrunt-iam-assume-role-session-name", "terragrunt-iam-role-session-name"},
			mockOptionsWithIamAssumeRoleSessionName(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, "terragrunt-iam-role-session-name"),
			nil,
		},

		{
			[]string{"--terragrunt-config", fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath), "--terragrunt-non-interactive"},
			mockOptions(t, fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath), workingDir, []string{}, true, "", false, false, defaultLogLevel, false),
			nil,
		},

		{
			[]string{"--foo", "--terragrunt-config", fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath), "bar", "--terragrunt-non-interactive", "--baz", "--terragrunt-working-dir", "/some/path", "--terragrunt-source", "github.com/foo/bar//baz?ref=1.0.3"},
			mockOptions(t, fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath), "/some/path", []string{"--foo", "bar", "--baz"}, true, "github.com/foo/bar//baz?ref=1.0.3", false, false, defaultLogLevel, false),
			nil,
		},

		// Adding the --terragrunt-log-level flag should result in DebugLevel configured
		{
			[]string{"--terragrunt-log-level", "debug"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, false, logrus.DebugLevel, false),
			nil,
		},
		{
			[]string{"--terragrunt-config"},
			nil,
			ArgMissingValue("terragrunt-config"),
		},

		{
			[]string{"--terragrunt-working-dir"},
			nil,
			ArgMissingValue("terragrunt-working-dir"),
		},

		{
			[]string{"--foo", "bar", "--terragrunt-config"},
			nil,
			ArgMissingValue("terragrunt-config"),
		},
		{
			[]string{"--terragrunt-debug"},
			mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false, "", false, false, defaultLogLevel, true),
			nil,
		},
	}

	for _, testCase := range testCases {
		actualOptions, actualErr := parseTerragruntOptionsFromArgs("0.0", testCase.args, &bytes.Buffer{}, &bytes.Buffer{})
		if testCase.expectedErr != nil {
			assert.True(t, errors.IsError(actualErr, testCase.expectedErr), "Expected error %v but got error %v", testCase.expectedErr, actualErr)
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
	assert.Equal(t, expected.AwsProviderPatchOverrides, actual.AwsProviderPatchOverrides, msgAndArgs...)
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

func mockOptionsWithIamAssumeRoleDuration(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, IamAssumeRoleDuration int64) *options.TerragruntOptions {
	opts := mockOptions(t, terragruntConfigPath, workingDir, terraformCliArgs, nonInteractive, terragruntSource, ignoreDependencyErrors, false, defaultLogLevel, false)
	opts.OriginalIAMRoleOptions.AssumeRoleDuration = IamAssumeRoleDuration
	opts.IAMRoleOptions.AssumeRoleDuration = IamAssumeRoleDuration

	return opts
}

func mockOptionsWithIamAssumeRoleSessionName(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool, terragruntSource string, ignoreDependencyErrors bool, IamAssumeRoleSessionName string) *options.TerragruntOptions {
	opts := mockOptions(t, terragruntConfigPath, workingDir, terraformCliArgs, nonInteractive, terragruntSource, ignoreDependencyErrors, false, defaultLogLevel, false)
	opts.OriginalIAMRoleOptions.AssumeRoleSessionName = IamAssumeRoleSessionName
	opts.IAMRoleOptions.AssumeRoleSessionName = IamAssumeRoleSessionName

	return opts
}

func mockOptionsWithOverrideAttrs(t *testing.T, terragruntConfigPath string, workingDir string, terraformCliArgs []string, overrideAttrs map[string]string) *options.TerragruntOptions {
	opts := mockOptions(t, terragruntConfigPath, workingDir, terraformCliArgs, false, "", false, false, defaultLogLevel, false)
	opts.AwsProviderPatchOverrides = overrideAttrs
	return opts
}

func TestFilterTerragruntArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args     []string
		expected []string
	}{
		{[]string{}, []string{}},
		{[]string{"foo", "--bar"}, []string{"foo", "--bar"}},
		{[]string{"foo", "--terragrunt-config", fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath)}, []string{"foo"}},
		{[]string{"foo", "--terragrunt-non-interactive"}, []string{"foo"}},
		{[]string{"foo", "--terragrunt-debug"}, []string{"foo"}},
		{[]string{"foo", "--terragrunt-non-interactive", "--bar", "--terragrunt-working-dir", "/some/path", "--baz", "--terragrunt-config", fmt.Sprintf("/some/path/%s", config.DefaultTerragruntConfigPath)}, []string{"foo", "--bar", "--baz"}},
		{[]string{"apply-all", "foo", "bar"}, []string{"foo", "bar"}},
		{[]string{"foo", "destroy-all", "--foo", "--bar"}, []string{"foo", "--foo", "--bar"}},
	}

	for _, testCase := range testCases {
		actual := filterTerragruntArgs(testCase.args)
		assert.Equal(t, testCase.expected, actual, "For args %v", testCase.args)
	}
}

func TestParseMultiStringArg(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args         []string
		argName      string
		defaultValue []string
		expectedVals []string
		expectedErr  error
	}{
		{[]string{"apply-all", "--foo", "bar"}, "foo", []string{"default_bar"}, []string{"bar"}, nil},
		{[]string{"apply-all", "--test", "bar"}, "foo", []string{"default_bar"}, []string{"default_bar"}, nil},
		{[]string{"plan-all", "--test", "--foo", "bar1", "--foo", "bar2"}, "foo", []string{"default_bar"}, []string{"bar1", "bar2"}, nil},
		{[]string{"plan-all", "--test", "value", "--foo", "bar1", "--foo"}, "foo", []string{"default_bar"}, nil, ArgMissingValue("foo")},
	}

	for _, testCase := range testCases {
		actual, actualErr := parseMultiStringArg(testCase.args, testCase.argName, testCase.defaultValue)

		if testCase.expectedErr != nil {
			assert.True(t, errors.IsError(actualErr, testCase.expectedErr), "Expected error %v but got error %v", testCase.expectedErr, actualErr)
		} else {
			assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
			assert.Equal(t, testCase.expectedVals, actual, "For args %v", testCase.args)
		}
	}
}

func TestParseMutliStringKeyValueArg(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args         []string
		argName      string
		defaultValue map[string]string
		expectedVals map[string]string
		expectedErr  error
	}{
		{[]string{"aws-provider-patch"}, "foo", nil, nil, nil},
		{[]string{"aws-provider-patch"}, "foo", map[string]string{"default": "value"}, map[string]string{"default": "value"}, nil},
		{[]string{"aws-provider-patch", "--other", "arg"}, "foo", map[string]string{"default": "value"}, map[string]string{"default": "value"}, nil},
		{[]string{"aws-provider-patch", "--foo", "key=value"}, "foo", map[string]string{"default": "value"}, map[string]string{"key": "value"}, nil},
		{[]string{"aws-provider-patch", "--foo", "key1=value1", "--foo", "key2=value2", "--foo", "key3=value3"}, "foo", map[string]string{"default": "value"}, map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"}, nil},
		{[]string{"aws-provider-patch", "--foo", "invalidvalue"}, "foo", map[string]string{"default": "value"}, nil, util.InvalidKeyValue("invalidvalue")},
	}

	for _, testCase := range testCases {
		actual, actualErr := parseMutliStringKeyValueArg(testCase.args, testCase.argName, testCase.defaultValue)

		if testCase.expectedErr != nil {
			assert.True(t, errors.IsError(actualErr, testCase.expectedErr), "Expected error %v but got error %v", testCase.expectedErr, actualErr)
		} else {
			assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
			assert.Equal(t, testCase.expectedVals, actual, "For args %v", testCase.args)
		}
	}
}

func TestParseEnvironmentVariables(t *testing.T) {
	testCases := []struct {
		environmentVariables []string
		expectedVariables    map[string]string
	}{
		{
			[]string{},
			map[string]string{},
		},
		{
			[]string{"foobar"},
			map[string]string{},
		},
		{
			[]string{"foo=bar"},
			map[string]string{"foo": "bar"},
		},
		{
			[]string{"foo=bar", "goo=gar"},
			map[string]string{"foo": "bar", "goo": "gar"},
		},
		{
			[]string{"foo=bar   "},
			map[string]string{"foo": "bar   "},
		},
		{
			[]string{"foo   =bar   "},
			map[string]string{"foo": "bar   "},
		},
		{
			[]string{"foo=composite=bar"},
			map[string]string{"foo": "composite=bar"},
		},
	}

	for _, testCase := range testCases {
		actualVariables := parseEnvironmentVariables(testCase.environmentVariables)
		assert.Equal(t, testCase.expectedVariables, actualVariables)
	}
}

func TestTerragruntHelp(t *testing.T) {
	app := CreateTerragruntCli("0.0", os.Stdout, os.Stderr)
	output := &bytes.Buffer{}
	app.Writer = output

	testCases := []struct {
		args     []string
		expected string
	}{
		{[]string{"terragrunt", "--help"}, app.UsageText},
		{[]string{"terragrunt", "-help"}, app.UsageText},
		{[]string{"terragrunt", "-h"}, app.UsageText},
		// TODO no support for showing command help texts
		//{[]string{"terragrunt", "plan-all", "--help"}, app.UsageText},
	}

	for _, testCase := range testCases {
		err := app.Run(testCase.args)

		require.NoError(t, err)
		if !strings.Contains(output.String(), testCase.expected) {
			t.Errorf("expected output to include help text; got: %q", output.String())
		}
	}
}

func TestTerraformHelp(t *testing.T) {
	app := CreateTerragruntCli("0.0", os.Stdout, os.Stderr)
	output := &bytes.Buffer{}
	app.Writer = output

	testCases := []struct {
		args     []string
		expected string
	}{
		{[]string{"terragrunt", "plan", "--help"}, "Usage: terraform .* plan"},
		{[]string{"terragrunt", "apply", "-help"}, "Usage: terraform .* apply"},
		{[]string{"terragrunt", "apply", "-h"}, "Usage: terraform .* apply"},
	}

	for _, testCase := range testCases {
		err := app.Run(testCase.args)
		require.NoError(t, err)

		expectedRegex, err := regexp.Compile(testCase.expected)
		require.NoError(t, err)

		assert.Regexp(t, expectedRegex, output.String())
	}
}

func TestTerraformHelp_wrongHelpFlag(t *testing.T) {
	app := CreateTerragruntCli("0.0", os.Stdout, os.Stderr)

	output := &bytes.Buffer{}
	app.Writer = output

	err := app.Run([]string{"terragrunt", "plan", "help"})
	require.Error(t, err)
}

func TestFilterTerraformExtraArgs(t *testing.T) {
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	workingDir = filepath.ToSlash(workingDir)

	temporaryFile := createTempFile(t)

	testCases := []struct {
		options      *options.TerragruntOptions
		extraArgs    config.TerraformExtraArguments
		expectedArgs []string
	}{
		// Standard scenario
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"apply", "plan", "destroy"}, []string{}, []string{}),
			[]string{"--foo", "bar"},
		},
		// optional existing var file
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"apply", "plan"}, []string{}, []string{temporaryFile}),
			[]string{"--foo", "bar", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// required var file + optional existing var file
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"apply", "plan"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// non existing required var file + non existing optional var file
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"apply", "plan"}, []string{"required.tfvars"}, []string{"optional.tfvars"}),
			[]string{"--foo", "bar", "-var-file=required.tfvars"},
		},
		// plan providing a folder, var files should stay included
		{
			mockCmdOptions(t, workingDir, []string{"plan", workingDir}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"plan", "apply"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// apply providing a folder, var files should stay included
		{
			mockCmdOptions(t, workingDir, []string{"apply", workingDir}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "-var='key=value'"}, []string{"plan", "apply"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "-var-file=test.tfvars", "-var='key=value'", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// apply providing a file, no var files included
		{
			mockCmdOptions(t, workingDir, []string{"apply", temporaryFile}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "apply"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "foo"},
		},

		// apply providing no params, var files should stay included
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "apply"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// apply with some parameters, providing a file => no var files included
		{
			mockCmdOptions(t, workingDir, []string{"apply", "-no-color", "-foo", temporaryFile}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "apply"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "foo"},
		},
		// destroy providing a folder, var files should stay included
		{
			mockCmdOptions(t, workingDir, []string{"destroy", workingDir}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "-var='key=value'"}, []string{"plan", "destroy"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "-var-file=test.tfvars", "-var='key=value'", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// destroy providing a file, no var files included
		{
			mockCmdOptions(t, workingDir, []string{"destroy", temporaryFile}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "destroy"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "foo"},
		},

		// destroy providing no params, var files should stay included
		{
			mockCmdOptions(t, workingDir, []string{"destroy"}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "destroy"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo", "-var-file=required.tfvars", fmt.Sprintf("-var-file=%s", temporaryFile)},
		},
		// destroy with some parameters, providing a file => no var files included
		{
			mockCmdOptions(t, workingDir, []string{"destroy", "-no-color", "-foo", temporaryFile}),
			mockExtraArgs([]string{"--foo", "-var-file=test.tfvars", "bar", "-var='key=value'", "foo"}, []string{"plan", "destroy"}, []string{"required.tfvars"}, []string{temporaryFile}),
			[]string{"--foo", "bar", "foo"},
		},

		// Command not included in commands list
		{
			mockCmdOptions(t, workingDir, []string{"apply"}),
			mockExtraArgs([]string{"--foo", "bar"}, []string{"plan", "destroy"}, []string{"required.tfvars"}, []string{"optional.tfvars"}),
			[]string{},
		},
	}
	for _, testCase := range testCases {
		config := config.TerragruntConfig{
			Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{testCase.extraArgs}},
		}

		out := filterTerraformExtraArgs(testCase.options, &config)

		assert.Equal(t, testCase.expectedArgs, out)
	}

}

func TestToTerraformEnvVars(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		description string
		vars        map[string]interface{}
		expected    map[string]string
	}{
		{
			"empty",
			map[string]interface{}{},
			map[string]string{},
		},
		{
			"string value",
			map[string]interface{}{"foo": "bar"},
			map[string]string{"TF_VAR_foo": `bar`},
		},
		{
			"int value",
			map[string]interface{}{"foo": 42},
			map[string]string{"TF_VAR_foo": `42`},
		},
		{
			"bool value",
			map[string]interface{}{"foo": true},
			map[string]string{"TF_VAR_foo": `true`},
		},
		{
			"list value",
			map[string]interface{}{"foo": []string{"a", "b", "c"}},
			map[string]string{"TF_VAR_foo": `["a","b","c"]`},
		},
		{
			"map value",
			map[string]interface{}{"foo": map[string]interface{}{"a": "b", "c": "d"}},
			map[string]string{"TF_VAR_foo": `{"a":"b","c":"d"}`},
		},
		{
			"nested map value",
			map[string]interface{}{"foo": map[string]interface{}{"a": []int{1, 2, 3}, "b": "c", "d": map[string]interface{}{"e": "f"}}},
			map[string]string{"TF_VAR_foo": `{"a":[1,2,3],"b":"c","d":{"e":"f"}}`},
		},
		{
			"multiple values",
			map[string]interface{}{"str": "bar", "int": 42, "bool": false, "list": []int{1, 2, 3}, "map": map[string]interface{}{"a": "b"}},
			map[string]string{"TF_VAR_str": `bar`, "TF_VAR_int": `42`, "TF_VAR_bool": `false`, "TF_VAR_list": `[1,2,3]`, "TF_VAR_map": `{"a":"b"}`},
		},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase
		t.Run(testCase.description, func(t *testing.T) {
			actual, err := toTerraformEnvVars(testCase.vars)
			require.NoError(t, err)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func createTempFile(t *testing.T) string {
	tmpFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s\n", err.Error())
	}

	return filepath.ToSlash(tmpFile.Name())
}

func mockCmdOptions(t *testing.T, workingDir string, terraformCliArgs []string) *options.TerragruntOptions {
	o := mockOptions(t, util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, terraformCliArgs, true, "", false, false, defaultLogLevel, false)
	return o
}

func mockExtraArgs(arguments, commands, requiredVarFiles, optionalVarFiles []string) config.TerraformExtraArguments {
	a := config.TerraformExtraArguments{
		Name:             "test",
		Arguments:        &arguments,
		Commands:         commands,
		RequiredVarFiles: &requiredVarFiles,
		OptionalVarFiles: &optionalVarFiles,
	}

	return a
}
