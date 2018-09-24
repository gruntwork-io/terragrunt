package config

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTerragruntConfigRemoteStateMinimalConfig(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  remote_state {
    backend = "s3"
  }
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.Empty(t, terragruntConfig.RemoteState.Config)
	}
}

func TestParseTerragruntConfigRemoteStateMissingBackend(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  remote_state {
  }
}
`

	_, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	assert.True(t, errors.IsError(err, remote.RemoteBackendMissing), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
}

func TestParseTerragruntConfigRemoteStateFullConfig(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  remote_state {
    backend = "s3"
    config {
      encrypt = true
      bucket = "my-bucket"
      key = "terraform.tfstate"
      region = "us-east-1"
    }
  }
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
	}
}

func TestParseTerragruntConfigDependenciesOnePath(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  dependencies {
    paths = ["../vpc"]
  }
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../vpc"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigDependenciesMultiplePaths(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  dependencies {
    paths = ["../vpc", "../mysql", "../backend-app"]
  }
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../vpc", "../mysql", "../backend-app"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigRemoteStateDynamoDbTerraformConfigAndDependenciesFullConfig(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  terraform {
    source = "foo"
  }

  remote_state {
    backend = "s3"
    config {
      encrypt = true
      bucket = "my-bucket"
      key = "terraform.tfstate"
      region = "us-east-1"
    }
  }

  dependencies {
    paths = ["../vpc", "../mysql", "../backend-app"]
  }
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Equal(t, "foo", terragruntConfig.Terraform.Source)
	}

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
	}

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../vpc", "../mysql", "../backend-app"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigRemoteStateDynamoDbTerraformConfigAndDependenciesFullConfigOldConfigFormat(t *testing.T) {
	t.Parallel()

	config := `
terraform {
  source = "foo"
}

remote_state {
  backend = "s3"
  config {
    encrypt = true
    bucket = "my-bucket"
    key = "terraform.tfstate"
    region = "us-east-1"
  }
}

dependencies {
  paths = ["../vpc", "../mysql", "../backend-app"]
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, OldTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Equal(t, "foo", terragruntConfig.Terraform.Source)
	}

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
	}

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../vpc", "../mysql", "../backend-app"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigInclude(t *testing.T) {
	t.Parallel()

	config :=
		fmt.Sprintf(`
terragrunt = {
  include {
    path = "../../../%s"
  }
}
`, DefaultTerragruntConfigPath)

	opts := options.TerragruntOptions{
		TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath,
		NonInteractive:       true,
	}

	terragruntConfig, err := parseConfigString(config, &opts, nil, opts.TerragruntConfigPath)
	if assert.Nil(t, err, "Unexpected error: %v", errors.PrintErrorWithStackTrace(err)) {
		assert.Nil(t, terragruntConfig.Terraform)

		if assert.NotNil(t, terragruntConfig.RemoteState) {
			assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
			assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
			assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
			assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
			assert.Equal(t, "child/sub-child/sub-sub-child/terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
			assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
		}
	}

}

func TestParseTerragruntConfigIncludeWithFindInParentFolders(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }
}
`

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath)

	terragruntConfig, err := parseConfigString(config, opts, nil, opts.TerragruntConfigPath)
	if assert.Nil(t, err, "Unexpected error: %v", errors.PrintErrorWithStackTrace(err)) {
		assert.Nil(t, terragruntConfig.Terraform)

		if assert.NotNil(t, terragruntConfig.RemoteState) {
			assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
			assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
			assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
			assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
			assert.Equal(t, "child/sub-child/sub-sub-child/terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
			assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
		}
	}

}

func TestParseTerragruntConfigIncludeOverrideRemote(t *testing.T) {
	t.Parallel()

	config :=
		fmt.Sprintf(`
terragrunt = {
  include {
    path = "../../../%s"
  }

  # Configure Terragrunt to automatically store tfstate files in an S3 bucket
  remote_state {
    backend = "s3"
    config {
      encrypt = false
      bucket = "override"
      key = "override"
      region = "override"
    }
  }
}
`, DefaultTerragruntConfigPath)

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath)

	terragruntConfig, err := parseConfigString(config, opts, nil, opts.TerragruntConfigPath)
	if assert.Nil(t, err, "Unexpected error: %v", errors.PrintErrorWithStackTrace(err)) {
		assert.Nil(t, terragruntConfig.Terraform)

		if assert.NotNil(t, terragruntConfig.RemoteState) {
			assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
			assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
			assert.Equal(t, false, terragruntConfig.RemoteState.Config["encrypt"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.Config["bucket"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.Config["key"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.Config["region"])
		}
	}

}

func TestParseTerragruntConfigIncludeOverrideAll(t *testing.T) {
	t.Parallel()

	config :=
		fmt.Sprintf(`
terragrunt = {
  include {
    path = "../../../%s"
  }

  terraform {
    source = "foo"
  }

  # Configure Terragrunt to automatically store tfstate files in an S3 bucket
  remote_state {
    backend = "s3"
    config {
      encrypt = false
      bucket = "override"
      key = "override"
      region = "override"
    }
  }

  dependencies {
    paths = ["override"]
  }
}
`, DefaultTerragruntConfigPath)

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath)

	terragruntConfig, err := parseConfigString(config, opts, nil, opts.TerragruntConfigPath)
	if assert.Nil(t, err, "Unexpected error: %v", errors.PrintErrorWithStackTrace(err)) {
		if assert.NotNil(t, terragruntConfig.Terraform) {
			assert.Equal(t, "foo", terragruntConfig.Terraform.Source)
		}

		if assert.NotNil(t, terragruntConfig.RemoteState) {
			assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
			assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
			assert.Equal(t, false, terragruntConfig.RemoteState.Config["encrypt"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.Config["bucket"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.Config["key"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.Config["region"])
		}

		assert.Equal(t, []string{"override"}, terragruntConfig.Dependencies.Paths)
	}

}

func TestParseTerragruntConfigTwoLevels(t *testing.T) {
	t.Parallel()

	configPath := "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/" + DefaultTerragruntConfigPath

	config, err := util.ReadFileAsString(configPath)
	if err != nil {
		t.Fatal(err)
	}

	opts := mockOptionsForTestWithConfigPath(t, configPath)

	_, actualErr := parseConfigString(config, opts, nil, configPath)
	expectedErr := TooManyLevelsOfInheritance{
		ConfigPath:             configPath,
		FirstLevelIncludePath:  "../" + DefaultTerragruntConfigPath,
		SecondLevelIncludePath: "../" + DefaultTerragruntConfigPath,
	}
	assert.True(t, errors.IsError(actualErr, expectedErr), "Expected error %v but got %v", expectedErr, actualErr)
}

func TestParseTerragruntConfigThreeLevels(t *testing.T) {
	t.Parallel()

	configPath := "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath

	config, err := util.ReadFileAsString(configPath)
	if err != nil {
		t.Fatal(err)
	}

	opts := mockOptionsForTestWithConfigPath(t, configPath)

	_, actualErr := parseConfigString(config, opts, nil, configPath)
	expectedErr := TooManyLevelsOfInheritance{
		ConfigPath:             configPath,
		FirstLevelIncludePath:  "../" + DefaultTerragruntConfigPath,
		SecondLevelIncludePath: "../" + DefaultTerragruntConfigPath,
	}
	assert.True(t, errors.IsError(actualErr, expectedErr), "Expected error %v but got %v", expectedErr, actualErr)
}

func TestParseTerragruntConfigEmptyConfig(t *testing.T) {
	t.Parallel()

	config := ``

	_, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	assert.True(t, errors.IsError(err, CouldNotResolveTerragruntConfigInFile(DefaultTerragruntConfigPath)))
}

func TestParseTerragruntConfigEmptyConfigOldConfig(t *testing.T) {
	t.Parallel()

	config := ``

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, OldTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
}

func TestMergeConfigIntoIncludedConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		config         *TerragruntConfig
		includedConfig *TerragruntConfig
		expected       *TerragruntConfig
	}{
		{
			&TerragruntConfig{},
			nil,
			&TerragruntConfig{},
		},
		{
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "foo"}},
			nil,
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "foo"}},
		},
		{
			&TerragruntConfig{},
			&TerragruntConfig{},
			&TerragruntConfig{},
		},
		{
			&TerragruntConfig{},
			&TerragruntConfig{Terraform: &TerraformConfig{Source: "foo"}},
			&TerragruntConfig{Terraform: &TerraformConfig{Source: "foo"}},
		},
		{
			&TerragruntConfig{},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &TerraformConfig{Source: "foo"}},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &TerraformConfig{Source: "foo"}},
		},
		{
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "foo"}, Terraform: &TerraformConfig{Source: "foo"}},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &TerraformConfig{Source: "bar"}},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "foo"}, Terraform: &TerraformConfig{Source: "foo"}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{Source: "foo"}},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &TerraformConfig{Source: "bar"}},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &TerraformConfig{Source: "foo"}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "childArgs"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{}},
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "childArgs"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "childArgs"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "parentArgs"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "parentArgs"}, TerraformExtraArguments{Name: "childArgs"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "overrideArgs", Arguments: []string{"-child"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "overrideArgs", Arguments: []string{"-parent"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "overrideArgs", Arguments: []string{"-child"}}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: nil},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: nil},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "parentHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "parentHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "parentHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "parentHooks"}, Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"parent-apply"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "parentHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "parentHooks"}, Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"parent-apply"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooksPlusMore", Commands: []string{"child-apply"}}, Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooksPlusMore", Commands: []string{"parent-apply"}}, Hook{Name: "parentHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooksPlusMore", Commands: []string{"child-apply"}}, Hook{Name: "parentHooks"}, Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideWithEmptyHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideWithEmptyHooks", Commands: []string{"parent-apply"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideWithEmptyHooks"}}}},
		},
	}

	for _, testCase := range testCases {
		actual, err := mergeConfigWithIncludedConfig(testCase.config, testCase.includedConfig, mockOptionsForTest(t))
		if assert.Nil(t, err, "Unexpected error for config %v and includeConfig %v: %v", testCase.config, testCase.includedConfig, err) {
			assert.Equal(t, testCase.expected, actual, "For config %v and includeConfig %v", testCase.config, testCase.includedConfig)
		}
	}
}

func TestParseTerragruntConfigTerraformNoSource(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  terraform {
  }
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Empty(t, terragruntConfig.Terraform.Source)
	}
}

func TestParseTerragruntConfigTerraformWithSource(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  terraform {
    source = "foo"
  }
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Equal(t, "foo", terragruntConfig.Terraform.Source)
	}
}

func TestParseTerragruntConfigTerraformWithExtraArguments(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  terraform {
    extra_arguments "secrets" {
      arguments = [
        "-var-file=terraform.tfvars",
        "-var-file=terraform-secret.tfvars"
      ]
			commands = ["${get_terraform_commands_that_need_vars()}"]
			env_vars = {
				TEST_VAR = "value"
			}
    }
  }
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Equal(t, "secrets", terragruntConfig.Terraform.ExtraArgs[0].Name)
		assert.Equal(t,
			[]string{
				"-var-file=terraform.tfvars",
				"-var-file=terraform-secret.tfvars",
			},
			terragruntConfig.Terraform.ExtraArgs[0].Arguments)
		assert.Equal(t,
			TERRAFORM_COMMANDS_NEED_VARS,
			terragruntConfig.Terraform.ExtraArgs[0].Commands)

		assert.Equal(t,
			map[string]string{"TEST_VAR": "value"},
			terragruntConfig.Terraform.ExtraArgs[0].EnvVars)
	}
}

func TestParseTerragruntConfigTerraformWithMultipleExtraArguments(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  terraform {
    extra_arguments "json_output" {
      arguments = [
        "-json"
      ]
      commands = [
        "output"
      ]
    }

    extra_arguments "fmt_diff" {
      arguments = [
        "-diff=true"
      ]
      commands = [
        "fmt"
      ]
    }

    extra_arguments "required_tfvars" {
      required_var_files = [
        "file1.tfvars",
				"file2.tfvars"
      ]
      commands = ["${get_terraform_commands_that_need_vars()}"]
    }

    extra_arguments "optional_tfvars" {
      optional_var_files = [
        "opt1.tfvars",
				"opt2.tfvars"
      ]
      commands = ["${get_terraform_commands_that_need_vars()}"]
    }
  }
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Equal(t, "json_output", terragruntConfig.Terraform.ExtraArgs[0].Name)
		assert.Equal(t, []string{"-json"}, terragruntConfig.Terraform.ExtraArgs[0].Arguments)
		assert.Equal(t, []string{"output"}, terragruntConfig.Terraform.ExtraArgs[0].Commands)
		assert.Equal(t, "fmt_diff", terragruntConfig.Terraform.ExtraArgs[1].Name)
		assert.Equal(t, []string{"-diff=true"}, terragruntConfig.Terraform.ExtraArgs[1].Arguments)
		assert.Equal(t, []string{"fmt"}, terragruntConfig.Terraform.ExtraArgs[1].Commands)
		assert.Equal(t, "required_tfvars", terragruntConfig.Terraform.ExtraArgs[2].Name)
		assert.Equal(t, []string{"file1.tfvars", "file2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[2].RequiredVarFiles)
		assert.Equal(t, TERRAFORM_COMMANDS_NEED_VARS, terragruntConfig.Terraform.ExtraArgs[2].Commands)
		assert.Equal(t, "optional_tfvars", terragruntConfig.Terraform.ExtraArgs[3].Name)
		assert.Equal(t, []string{"opt1.tfvars", "opt2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[3].OptionalVarFiles)
		assert.Equal(t, TERRAFORM_COMMANDS_NEED_VARS, terragruntConfig.Terraform.ExtraArgs[3].Commands)
	}
}

func TestFindConfigFilesInPathNone(t *testing.T) {
	t.Parallel()

	expected := []string{}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/none", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathOneNewConfig(t *testing.T) {
	t.Parallel()

	expected := []string{"../test/fixture-config-files/one-new-config/subdir/terraform.tfvars"}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/one-new-config", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathOneOldConfig(t *testing.T) {
	t.Parallel()

	expected := []string{"../test/fixture-config-files/one-old-config/subdir/.terragrunt"}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/one-old-config", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathMultipleConfigs(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/multiple-configs/terraform.tfvars",
		"../test/fixture-config-files/multiple-configs/subdir-2/subdir/.terragrunt",
		"../test/fixture-config-files/multiple-configs/subdir-3/terraform.tfvars",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/multiple-configs", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerragruntCache(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/ignore-cached-config/terraform.tfvars",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/ignore-cached-config", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesIgnoresDownloadDir(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/multiple-configs/terraform.tfvars",
		"../test/fixture-config-files/multiple-configs/subdir-3/terraform.tfvars",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)
	terragruntOptions.DownloadDir = "../test/fixture-config-files/multiple-configs/subdir-2"

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/multiple-configs", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func mockOptionsForTestWithConfigPath(t *testing.T, configPath string) *options.TerragruntOptions {
	opts, err := options.NewTerragruntOptionsForTest(configPath)
	if err != nil {
		t.Fatalf("Faile to create TerragruntOptions: %v", err)
	}
	return opts
}

func mockOptionsForTest(t *testing.T) *options.TerragruntOptions {
	return mockOptionsForTestWithConfigPath(t, "test-time-mock")
}

func TestParseTerragruntConfigPreventDestroyTrue(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  prevent_destroy = true
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Equal(t, true, terragruntConfig.PreventDestroy)
}

func TestParseTerragruntConfigPreventDestroyFalse(t *testing.T) {
	t.Parallel()

	config := `
terragrunt = {
  prevent_destroy = false
}
`

	terragruntConfig, err := parseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Equal(t, false, terragruntConfig.PreventDestroy)
}
