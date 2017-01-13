package cli

import (
	"testing"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/config"
	"os"
	"path/filepath"
	"github.com/gruntwork-io/terragrunt/util"
)

func TestParseTerragruntOptionsFromArgs(t *testing.T) {
	t.Parallel()

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		args 		[]string
		expectedOptions *options.TerragruntOptions
		expectedErr     error
	}{
		{
			[]string{},
			mockOptions(util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, false),
			nil,
		},

		{
			[]string{"foo", "bar"},
			mockOptions(util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"foo", "bar"}, false),
			nil,
		},

		{
			[]string{"--foo", "--bar"},
			mockOptions(util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"--foo", "--bar"}, false),
			nil,
		},

		{
			[]string{"--foo", "apply", "--bar"},
			mockOptions(util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{"--foo", "apply", "--bar"}, false),
			nil,
		},

		{
			[]string{"--terragrunt-non-interactive"},
			mockOptions(util.JoinPath(workingDir, config.DefaultTerragruntConfigPath), workingDir, []string{}, true),
			nil,
		},

		{
			[]string{"--terragrunt-config", "/some/path/.terragrunt"},
			mockOptions("/some/path/.terragrunt", workingDir, []string{}, false),
			nil,
		},

		{
			[]string{"--terragrunt-working-dir", "/some/path"},
			mockOptions(util.JoinPath("/some/path", config.DefaultTerragruntConfigPath), "/some/path", []string{}, false),
			nil,
		},

		{
			[]string{"--terragrunt-config", "/some/path/.terragrunt", "--terragrunt-non-interactive"},
			mockOptions("/some/path/.terragrunt", workingDir, []string{}, true),
			nil,
		},

		{
			[]string{"--foo", "--terragrunt-config", "/some/path/.terragrunt", "bar", "--terragrunt-non-interactive", "--baz", "--terragrunt-working-dir", "/some/path"},
			mockOptions("/some/path/.terragrunt", "/some/path", []string{"--foo", "bar", "--baz"}, true),
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
	}

	for _, testCase := range testCases {
		actualOptions, actualErr := parseTerragruntOptionsFromArgs(testCase.args)
		if testCase.expectedErr != nil {
			assert.True(t, errors.IsError(actualErr, testCase.expectedErr), "Expected error %v but got error %v", testCase.expectedErr, actualErr)
		} else {
			assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
			assertOptionsEqual(t, *testCase.expectedOptions, *actualOptions)
		}
	}
}

// We can't do a direct comparison between TerragruntOptions objects because we can't compare Logger or RunTerragrunt
// instances. Therefore, we have to manually check everything else.
func assertOptionsEqual(t *testing.T, expected options.TerragruntOptions, actual options.TerragruntOptions) {
	assert.NotNil(t, expected.Logger)
	assert.NotNil(t, actual.Logger)

	assert.Equal(t, expected.TerragruntConfigPath, actual.TerragruntConfigPath)
	assert.Equal(t, expected.NonInteractive, actual.NonInteractive)
	assert.Equal(t, expected.TerraformCliArgs, actual.TerraformCliArgs)
	assert.Equal(t, expected.WorkingDir, actual.WorkingDir)
}

func mockOptions(terragruntConfigPath string, workingDir string, terraformCliArgs []string, nonInteractive bool) *options.TerragruntOptions {
	opts := options.NewTerragruntOptionsForTest(terragruntConfigPath)

	opts.WorkingDir = workingDir
	opts.TerraformCliArgs = terraformCliArgs
	opts.NonInteractive = nonInteractive

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
		{[]string{"foo", "--terragrunt-config", "/some/path/.terragrunt"}, []string{"foo"}},
		{[]string{"foo", "--terragrunt-non-interactive"}, []string{"foo"}},
		{[]string{"foo", "--terragrunt-non-interactive", "--bar", "--terragrunt-working-dir", "/some/path", "--baz", "--terragrunt-config", "/some/path/.terragrunt"}, []string{"foo", "--bar", "--baz"}},
		{[]string{"spin-up", "foo", "bar"}, []string{"foo", "bar"}},
		{[]string{"foo", "tear-down", "--foo", "--bar"}, []string{"foo", "--foo", "--bar"}},
	}

	for _, testCase := range testCases {
		actual := filterTerragruntArgs(testCase.args)
		assert.Equal(t, testCase.expected, actual, "For args %v", testCase.args)
	}
}