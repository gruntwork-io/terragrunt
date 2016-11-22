package cli

import (
	"testing"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/config"
)

func TestParseTerragruntOptionsFromArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args 		[]string
		expectedOptions *options.TerragruntOptions
		expectedErr     error
	}{
		{
			[]string{},
			&options.TerragruntOptions{
				TerragruntConfigPath: config.DefaultTerragruntConfigPath,
				NonInteractive: false,
				NonTerragruntArgs: []string{},
			},
			nil,
		},

		{
			[]string{"foo", "bar"},
			&options.TerragruntOptions{
				TerragruntConfigPath: config.DefaultTerragruntConfigPath,
				NonInteractive: false,
				NonTerragruntArgs: []string{"foo", "bar"},
			},
			nil,
		},

		{
			[]string{"--foo", "--bar"},
			&options.TerragruntOptions{
				TerragruntConfigPath: config.DefaultTerragruntConfigPath,
				NonInteractive: false,
				NonTerragruntArgs: []string{"--foo", "--bar"},
			},
			nil,
		},

		{
			[]string{"--foo", "apply", "--bar"},
			&options.TerragruntOptions{
				TerragruntConfigPath: config.DefaultTerragruntConfigPath,
				NonInteractive: false,
				NonTerragruntArgs: []string{"--foo", "apply", "--bar"},
			},
			nil,
		},

		{
			[]string{"--terragrunt-non-interactive"},
			&options.TerragruntOptions{
				TerragruntConfigPath: config.DefaultTerragruntConfigPath,
				NonInteractive: true,
				NonTerragruntArgs: []string{},
			},
			nil,
		},

		{
			[]string{"--terragrunt-config", "/some/path/.terragrunt"},
			&options.TerragruntOptions{
				TerragruntConfigPath: "/some/path/.terragrunt",
				NonInteractive: false,
				NonTerragruntArgs: []string{},
			},
			nil,
		},

		{
			[]string{"--terragrunt-config", "/some/path/.terragrunt", "--terragrunt-non-interactive"},
			&options.TerragruntOptions{
				TerragruntConfigPath: "/some/path/.terragrunt",
				NonInteractive: true,
				NonTerragruntArgs: []string{},
			},
			nil,
		},

		{
			[]string{"--foo", "--terragrunt-config", "/some/path/.terragrunt", "bar", "--terragrunt-non-interactive", "--baz"},
			&options.TerragruntOptions{
				TerragruntConfigPath: "/some/path/.terragrunt",
				NonInteractive: true,
				NonTerragruntArgs: []string{"--foo", "bar", "--baz"},
			},
			nil,
		},

		{
			[]string{"--terragrunt-config"},
			nil,
			MissingTerragruntConfigValue,
		},

		{
			[]string{"--foo", "bar", "--terragrunt-config"},
			nil,
			MissingTerragruntConfigValue,
		},
	}

	for _, testCase := range testCases {
		actualOptions, actualErr := parseTerragruntOptionsFromArgs(testCase.args)
		if testCase.expectedErr != nil {
			assert.True(t, errors.IsError(actualErr, testCase.expectedErr), "Expected error %v but got error %v", testCase.expectedErr, actualErr)
		} else {
			assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
			assert.Equal(t, testCase.expectedOptions, actualOptions, "Expected options %v but got %v", testCase.expectedOptions, actualOptions)
		}
	}
}
