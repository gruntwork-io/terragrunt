package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTerragruntConfigRemoteStateMinimalConfig(t *testing.T) {
	t.Parallel()

	config := `
remote_state {
  backend = "s3"
  config  = {}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)

	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.Empty(t, terragruntConfig.RemoteState.Config)
	}
}

func TestParseTerragruntConfigRemoteStateAttrMinimalConfig(t *testing.T) {
	t.Parallel()

	config := `
remote_state = {
  backend = "s3"
  config  = {}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)

	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.Empty(t, terragruntConfig.RemoteState.Config)
	}
}

func TestParseTerragruntJsonConfigRemoteStateMinimalConfig(t *testing.T) {
	t.Parallel()

	config := `
{
	"remote_state": {
		"backend": "s3",
		"config": {}
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntJsonConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.Empty(t, terragruntConfig.RemoteState.Config)
	}
}

func TestParseTerragruntHclConfigRemoteStateMissingBackend(t *testing.T) {
	t.Parallel()

	config := `
remote_state {}
`

	_, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Missing required argument; The argument \"backend\" is required")
}

func TestParseTerragruntHclConfigRemoteStateFullConfig(t *testing.T) {
	t.Parallel()

	config := `
remote_state {
	backend = "s3"
	config = {
  		encrypt = true
  		bucket = "my-bucket"
  		key = "terraform.tfstate"
  		region = "us-east-1"
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
	}
}

func TestParseTerragruntJsonConfigRemoteStateFullConfig(t *testing.T) {
	t.Parallel()

	config := `
{
	"remote_state":{
		"backend":"s3",
		"config":{
			"encrypt": true,
			"bucket": "my-bucket",
			"key": "terraform.tfstate",
			"region":"us-east-1"
		}
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntJsonConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
	}
}

func TestParseTerragruntHclConfigRetryConfiguration(t *testing.T) {
	t.Parallel()

	config := `
retry_max_attempts = 10
retry_sleep_interval_sec = 60
retryable_errors = [
    "My own little error",
    "Another one of my errors"
]
`
	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Empty(t, terragruntConfig.IamRole)

	assert.Equal(t, 10, *terragruntConfig.RetryMaxAttempts)
	assert.Equal(t, 60, *terragruntConfig.RetrySleepIntervalSec)

	if assert.NotNil(t, terragruntConfig.RetryableErrors) {
		assert.Equal(t, []string{"My own little error", "Another one of my errors"}, terragruntConfig.RetryableErrors)
	}
}

func TestParseTerragruntJsonConfigRetryConfiguration(t *testing.T) {
	t.Parallel()

	config := `
{
	"retry_max_attempts": 10,
	"retry_sleep_interval_sec": 60,
	"retryable_errors": [
        "My own little error"
	]
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntJsonConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Empty(t, terragruntConfig.IamRole)

	assert.Equal(t, *terragruntConfig.RetryMaxAttempts, 10)
	assert.Equal(t, *terragruntConfig.RetrySleepIntervalSec, 60)

	if assert.NotNil(t, terragruntConfig.RetryableErrors) {
		assert.Equal(t, []string{"My own little error"}, terragruntConfig.RetryableErrors)
	}
}

func TestParseIamRole(t *testing.T) {
	t.Parallel()

	config := `iam_role = "terragrunt-iam-role"`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Nil(t, terragruntConfig.RetryableErrors)

	assert.Equal(t, "terragrunt-iam-role", terragruntConfig.IamRole)
}

func TestParseIamAssumeRoleDuration(t *testing.T) {
	t.Parallel()

	config := `iam_assume_role_duration = 36000`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Nil(t, terragruntConfig.RetryableErrors)

	assert.Equal(t, int64(36000), *terragruntConfig.IamAssumeRoleDuration)
}

func TestParseIamAssumeRoleSessionName(t *testing.T) {
	t.Parallel()

	config := `iam_assume_role_session_name = "terragrunt-iam-assume-role-session-name"`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Nil(t, terragruntConfig.RetryableErrors)

	assert.Equal(t, "terragrunt-iam-assume-role-session-name", terragruntConfig.IamAssumeRoleSessionName)
}

func TestParseTerragruntConfigDependenciesOnePath(t *testing.T) {
	t.Parallel()

	config := `
dependencies {
	paths = ["../test/fixture-parent-folders/multiple-terragrunt-in-parents"]
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RetryableErrors)

	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixture-parent-folders/multiple-terragrunt-in-parents"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigDependenciesMultiplePaths(t *testing.T) {
	t.Parallel()

	config := `
dependencies {
	paths = ["../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"]
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigRemoteStateDynamoDbTerraformConfigAndDependenciesFullConfig(t *testing.T) {
	t.Parallel()

	config := `
terraform {
	source = "foo"
}

remote_state {
	backend = "s3"
	config = {
		encrypt = true
		bucket = "my-bucket"
		key = "terraform.tfstate"
		region = "us-east-1"
	}
}

dependencies {
	paths = ["../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"]
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
	}

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntJsonConfigRemoteStateDynamoDbTerraformConfigAndDependenciesFullConfig(t *testing.T) {
	t.Parallel()

	config := `
{
	"terraform": {
		"source": "foo"
	},
	"remote_state": {
		"backend": "s3",
		"config": {
			"encrypt": true,
			"bucket": "my-bucket",
			"key": "terraform.tfstate",
			"region": "us-east-1"
		}
	},
	"dependencies":{
		"paths": ["../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"]
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntJsonConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)
	assert.Nil(t, terragruntConfig.RetryableErrors)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
		assert.NotEmpty(t, terragruntConfig.RemoteState.Config)
		assert.Equal(t, true, terragruntConfig.RemoteState.Config["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.Config["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.Config["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.Config["region"])
	}

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixture", "../test/fixture-dirs", "../test/fixture-inputs"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigInclude(t *testing.T) {
	t.Parallel()

	config :=
		fmt.Sprintf(`
include {
	path = "../../../%s"
}
`, DefaultTerragruntConfigPath)

	opts := options.TerragruntOptions{
		TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/" + DefaultTerragruntConfigPath,
		NonInteractive:       true,
		Logger:               util.CreateLogEntry("", util.GetDefaultLogLevel()),
	}

	terragruntConfig, err := ParseConfigString(config, &opts, nil, opts.TerragruntConfigPath, nil)
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
include {
	path = find_in_parent_folders()
}
`

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath)

	terragruntConfig, err := ParseConfigString(config, opts, nil, opts.TerragruntConfigPath, nil)
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
include {
	path = "../../../%s"
}

# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
	backend = "s3"
	config = {
		encrypt = false
		bucket = "override"
		key = "override"
		region = "override"
	}
}
`, DefaultTerragruntConfigPath)

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath)

	terragruntConfig, err := ParseConfigString(config, opts, nil, opts.TerragruntConfigPath, nil)
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
include {
	path = "../../../%s"
}

terraform {
	source = "foo"
}

# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
	backend = "s3"
	config = {
		encrypt = false
		bucket = "override"
		key = "override"
		region = "override"
	}
}

dependencies {
	paths = ["override"]
}
`, DefaultTerragruntConfigPath)

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntConfigPath)

	terragruntConfig, err := ParseConfigString(config, opts, nil, opts.TerragruntConfigPath, nil)
	require.NoError(t, err, "Unexpected error: %v", errors.PrintErrorWithStackTrace(err))

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)

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

func TestParseTerragruntJsonConfigIncludeOverrideAll(t *testing.T) {
	t.Parallel()

	config :=
		fmt.Sprintf(`
{
	"include":{
		"path": "../../../%s"
	},
	"terraform":{
		"source": "foo"
	},
	"remote_state":{
		"backend": "s3",
		"config":{
			"encrypt": false,
			"bucket": "override",
			"key": "override",
			"region": "override"
		}
	},
	"dependencies":{
		"paths": ["override"]
	}
}
`, DefaultTerragruntConfigPath)

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixture-parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+DefaultTerragruntJsonConfigPath)

	terragruntConfig, err := ParseConfigString(config, opts, nil, opts.TerragruntConfigPath, nil)
	require.NoError(t, err, "Unexpected error: %v", errors.PrintErrorWithStackTrace(err))

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)

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

func TestParseTerragruntConfigTwoLevels(t *testing.T) {
	t.Parallel()

	configPath := "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/" + DefaultTerragruntConfigPath

	config, err := util.ReadFileAsString(configPath)
	if err != nil {
		t.Fatal(err)
	}

	opts := mockOptionsForTestWithConfigPath(t, configPath)

	_, actualErr := ParseConfigString(config, opts, nil, configPath, nil)
	expectedErr := TooManyLevelsOfInheritance{
		ConfigPath:             configPath,
		FirstLevelIncludePath:  absPath(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/"+DefaultTerragruntConfigPath),
		SecondLevelIncludePath: absPath(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/"+DefaultTerragruntConfigPath),
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

	_, actualErr := ParseConfigString(config, opts, nil, configPath, nil)
	expectedErr := TooManyLevelsOfInheritance{
		ConfigPath:             configPath,
		FirstLevelIncludePath:  absPath(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+DefaultTerragruntConfigPath),
		SecondLevelIncludePath: absPath(t, "../test/fixture-parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+DefaultTerragruntConfigPath),
	}
	assert.True(t, errors.IsError(actualErr, expectedErr), "Expected error %v but got %v", expectedErr, actualErr)
}

func TestParseTerragruntConfigEmptyConfig(t *testing.T) {
	t.Parallel()

	config := ``

	cfg, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	assert.NoError(t, err)

	assert.Nil(t, cfg.Terraform)
	assert.Nil(t, cfg.RemoteState)
	assert.Nil(t, cfg.Dependencies)
	assert.Nil(t, cfg.PreventDestroy)
	assert.False(t, cfg.Skip)
	assert.Empty(t, cfg.IamRole)
	assert.Nil(t, cfg.RetryMaxAttempts)
	assert.Nil(t, cfg.RetrySleepIntervalSec)
	assert.Nil(t, cfg.RetryableErrors)
}

func TestParseTerragruntConfigEmptyConfigOldConfig(t *testing.T) {
	t.Parallel()

	config := ``

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
}

func TestParseTerragruntConfigTerraformNoSource(t *testing.T) {
	t.Parallel()

	config := `
terraform {}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	require.NotNil(t, terragruntConfig.Terraform)
	require.Nil(t, terragruntConfig.Terraform.Source)
}

func TestParseTerragruntConfigTerraformWithSource(t *testing.T) {
	t.Parallel()

	config := `
terraform {
	source = "foo"
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)
}

func TestParseTerragruntConfigTerraformWithExtraArguments(t *testing.T) {
	t.Parallel()

	config := `
terraform {
	extra_arguments "secrets" {
		arguments = [
			"-var-file=terraform.tfvars",
			"-var-file=terraform-secret.tfvars"
		]
		commands = get_terraform_commands_that_need_vars()
		env_vars = {
			TEST_VAR = "value"
		}
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Equal(t, "secrets", terragruntConfig.Terraform.ExtraArgs[0].Name)
		assert.Equal(t,
			&[]string{
				"-var-file=terraform.tfvars",
				"-var-file=terraform-secret.tfvars",
			},
			terragruntConfig.Terraform.ExtraArgs[0].Arguments)
		assert.Equal(t,
			TERRAFORM_COMMANDS_NEED_VARS,
			terragruntConfig.Terraform.ExtraArgs[0].Commands)

		assert.Equal(t,
			&map[string]string{"TEST_VAR": "value"},
			terragruntConfig.Terraform.ExtraArgs[0].EnvVars)
	}
}

func TestParseTerragruntConfigTerraformWithMultipleExtraArguments(t *testing.T) {
	t.Parallel()

	config := `
terraform {
	extra_arguments "json_output" {
		arguments = ["-json"]
		commands = ["output"]
	}

	extra_arguments "fmt_diff" {
		arguments = ["-diff=true"]
		commands = ["fmt"]
	}

	extra_arguments "required_tfvars" {
		required_var_files = [
			"file1.tfvars",
			"file2.tfvars"
		]
		commands = get_terraform_commands_that_need_vars()
	}

	extra_arguments "optional_tfvars" {
		optional_var_files = [
			"opt1.tfvars",
			"opt2.tfvars"
		]
		commands = get_terraform_commands_that_need_vars()
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Equal(t, "json_output", terragruntConfig.Terraform.ExtraArgs[0].Name)
		assert.Equal(t, &[]string{"-json"}, terragruntConfig.Terraform.ExtraArgs[0].Arguments)
		assert.Equal(t, []string{"output"}, terragruntConfig.Terraform.ExtraArgs[0].Commands)
		assert.Equal(t, "fmt_diff", terragruntConfig.Terraform.ExtraArgs[1].Name)
		assert.Equal(t, &[]string{"-diff=true"}, terragruntConfig.Terraform.ExtraArgs[1].Arguments)
		assert.Equal(t, []string{"fmt"}, terragruntConfig.Terraform.ExtraArgs[1].Commands)
		assert.Equal(t, "required_tfvars", terragruntConfig.Terraform.ExtraArgs[2].Name)
		assert.Equal(t, &[]string{"file1.tfvars", "file2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[2].RequiredVarFiles)
		assert.Equal(t, TERRAFORM_COMMANDS_NEED_VARS, terragruntConfig.Terraform.ExtraArgs[2].Commands)
		assert.Equal(t, "optional_tfvars", terragruntConfig.Terraform.ExtraArgs[3].Name)
		assert.Equal(t, &[]string{"opt1.tfvars", "opt2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[3].OptionalVarFiles)
		assert.Equal(t, TERRAFORM_COMMANDS_NEED_VARS, terragruntConfig.Terraform.ExtraArgs[3].Commands)
	}
}

func TestParseTerragruntJsonConfigTerraformWithMultipleExtraArguments(t *testing.T) {
	t.Parallel()

	config := `
{
	"terraform":{
		"extra_arguments":{
			"json_output":{
				"arguments": ["-json"],
				"commands": ["output"]
			},
			"fmt_diff":{
				"arguments": ["-diff=true"],
				"commands": ["fmt"]
			},
			"required_tfvars":{
				"required_var_files":[
					"file1.tfvars",
					"file2.tfvars"
				],
				"commands": "${get_terraform_commands_that_need_vars()}"
			},
			"optional_tfvars":{
				"optional_var_files":[
					"opt1.tfvars",
					"opt2.tfvars"
				],
				"commands": "${get_terraform_commands_that_need_vars()}"
			}
		}
	}
}
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntJsonConfigPath, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	if assert.NotNil(t, terragruntConfig.Terraform) {
		assert.Equal(t, "json_output", terragruntConfig.Terraform.ExtraArgs[0].Name)
		assert.Equal(t, &[]string{"-json"}, terragruntConfig.Terraform.ExtraArgs[0].Arguments)
		assert.Equal(t, []string{"output"}, terragruntConfig.Terraform.ExtraArgs[0].Commands)
		assert.Equal(t, "fmt_diff", terragruntConfig.Terraform.ExtraArgs[1].Name)
		assert.Equal(t, &[]string{"-diff=true"}, terragruntConfig.Terraform.ExtraArgs[1].Arguments)
		assert.Equal(t, []string{"fmt"}, terragruntConfig.Terraform.ExtraArgs[1].Commands)
		assert.Equal(t, "required_tfvars", terragruntConfig.Terraform.ExtraArgs[2].Name)
		assert.Equal(t, &[]string{"file1.tfvars", "file2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[2].RequiredVarFiles)
		assert.Equal(t, TERRAFORM_COMMANDS_NEED_VARS, terragruntConfig.Terraform.ExtraArgs[2].Commands)
		assert.Equal(t, "optional_tfvars", terragruntConfig.Terraform.ExtraArgs[3].Name)
		assert.Equal(t, &[]string{"opt1.tfvars", "opt2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[3].OptionalVarFiles)
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

func TestFindConfigFilesInPathOneConfig(t *testing.T) {
	t.Parallel()

	expected := []string{"../test/fixture-config-files/one-config/subdir/terragrunt.hcl"}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/one-config", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathOneJsonConfig(t *testing.T) {
	t.Parallel()

	expected := []string{"../test/fixture-config-files/one-json-config/subdir/terragrunt.hcl.json"}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/one-json-config", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathMultipleConfigs(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/multiple-configs/terragrunt.hcl",
		"../test/fixture-config-files/multiple-configs/subdir-2/subdir/terragrunt.hcl",
		"../test/fixture-config-files/multiple-configs/subdir-3/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/multiple-configs", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesInPathMultipleJsonConfigs(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/multiple-json-configs/terragrunt.hcl.json",
		"../test/fixture-config-files/multiple-json-configs/subdir-2/subdir/terragrunt.hcl.json",
		"../test/fixture-config-files/multiple-json-configs/subdir-3/terragrunt.hcl.json",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/multiple-json-configs", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesInPathMultipleMixedConfigs(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/multiple-mixed-configs/terragrunt.hcl.json",
		"../test/fixture-config-files/multiple-mixed-configs/subdir-2/subdir/terragrunt.hcl",
		"../test/fixture-config-files/multiple-mixed-configs/subdir-3/terragrunt.hcl.json",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/multiple-mixed-configs", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerragruntCache(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/ignore-cached-config/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/ignore-cached-config", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDir(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/ignore-terraform-data-dir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/.tf_data/modules/mod/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/.tf_data/modules/mod/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/ignore-terraform-data-dir", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDirEnv(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/ignore-terraform-data-dir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/.terraform/modules/mod/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)
	terragruntOptions.Env["TF_DATA_DIR"] = ".tf_data"

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/ignore-terraform-data-dir", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDirEnvPath(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/ignore-terraform-data-dir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/.tf_data/modules/mod/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl",
		"../test/fixture-config-files/ignore-terraform-data-dir/subdir/.terraform/modules/mod/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)
	terragruntOptions.Env["TF_DATA_DIR"] = "subdir/.tf_data"

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/ignore-terraform-data-dir", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDirEnvRoot(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	require.NoError(t, err)

	expected := []string{
		filepath.Join(cwd, "../test/fixture-config-files/ignore-terraform-data-dir/terragrunt.hcl"),
		filepath.Join(cwd, "../test/fixture-config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl"),
		filepath.Join(cwd, "../test/fixture-config-files/ignore-terraform-data-dir/subdir/.terraform/modules/mod/terragrunt.hcl"),
		filepath.Join(cwd, "../test/fixture-config-files/ignore-terraform-data-dir/subdir/.tf_data/modules/mod/terragrunt.hcl"),
	}
	workingDir := filepath.Join(cwd, "../test/fixture-config-files/ignore-terraform-data-dir/")
	terragruntOptions, err := options.NewTerragruntOptionsForTest(workingDir)
	require.NoError(t, err)
	terragruntOptions.Env["TF_DATA_DIR"] = filepath.Join(workingDir, ".tf_data")

	actual, err := FindConfigFilesInPath(workingDir, terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesIgnoresDownloadDir(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixture-config-files/multiple-configs/terragrunt.hcl",
		"../test/fixture-config-files/multiple-configs/subdir-3/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)
	terragruntOptions.DownloadDir = "../test/fixture-config-files/multiple-configs/subdir-2"

	actual, err := FindConfigFilesInPath("../test/fixture-config-files/multiple-configs", terragruntOptions)

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func mockOptionsForTestWithConfigPath(t *testing.T, configPath string) *options.TerragruntOptions {
	opts, err := options.NewTerragruntOptionsForTest(configPath)
	if err != nil {
		t.Fatalf("Failed to create TerragruntOptions: %v", err)
	}
	return opts
}

func mockOptionsForTest(t *testing.T) *options.TerragruntOptions {
	return mockOptionsForTestWithConfigPath(t, "test-time-mock")
}

func TestParseTerragruntConfigPreventDestroyTrue(t *testing.T) {
	t.Parallel()

	config := `
prevent_destroy = true
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Equal(t, true, *terragruntConfig.PreventDestroy)
}

func TestParseTerragruntConfigPreventDestroyFalse(t *testing.T) {
	t.Parallel()

	config := `
prevent_destroy = false
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Equal(t, false, *terragruntConfig.PreventDestroy)
}

func TestParseTerragruntConfigSkipTrue(t *testing.T) {
	t.Parallel()

	config := `
skip = true
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Equal(t, true, terragruntConfig.Skip)
}

func TestParseTerragruntConfigSkipFalse(t *testing.T) {
	t.Parallel()

	config := `
skip = false
`

	terragruntConfig, err := ParseConfigString(config, mockOptionsForTest(t), nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Equal(t, false, terragruntConfig.Skip)
}

func TestIncludeFunctionsWorkInChildConfig(t *testing.T) {
	config := `
include {
	path = find_in_parent_folders()
}
terraform {
	source = path_relative_to_include()
}
`
	opts := options.TerragruntOptions{
		TerragruntConfigPath: "../test/fixture-parent-folders/terragrunt-in-root/child/" + DefaultTerragruntConfigPath,
		NonInteractive:       true,
		MaxFoldersToCheck:    5,
		Logger:               util.CreateLogEntry("", util.GetDefaultLogLevel()),
	}

	terragruntConfig, err := ParseConfigString(config, &opts, nil, DefaultTerragruntConfigPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "child", *terragruntConfig.Terraform.Source)
}

func TestModuleDependenciesMerge(t *testing.T) {
	testCases := []struct {
		name     string
		target   []string
		source   []string
		expected []string
	}{
		{
			"MergeNil",
			[]string{"../vpc", "../sql"},
			nil,
			[]string{"../vpc", "../sql"},
		},
		{
			"MergeOne",
			[]string{"../vpc", "../sql"},
			[]string{"../services"},
			[]string{"../vpc", "../sql", "../services"},
		},
		{
			"MergeMany",
			[]string{"../vpc", "../sql"},
			[]string{"../services", "../groups"},
			[]string{"../vpc", "../sql", "../services", "../groups"},
		},
		{
			"MergeEmpty",
			[]string{"../vpc", "../sql"},
			[]string{},
			[]string{"../vpc", "../sql"},
		},
		{
			"MergeOneExisting",
			[]string{"../vpc", "../sql"},
			[]string{"../vpc"},
			[]string{"../vpc", "../sql"},
		},
		{
			"MergeAllExisting",
			[]string{"../vpc", "../sql"},
			[]string{"../vpc", "../sql"},
			[]string{"../vpc", "../sql"},
		},
		{
			"MergeSomeExisting",
			[]string{"../vpc", "../sql"},
			[]string{"../vpc", "../services"},
			[]string{"../vpc", "../sql", "../services"},
		},
	}

	for _, testCase := range testCases {
		// Capture range variable so that it is brought into the scope within the for loop, so that it is stable even
		// when subtests are run in parallel.
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			target := &ModuleDependencies{Paths: testCase.target}

			var source *ModuleDependencies = nil
			if testCase.source != nil {
				source = &ModuleDependencies{Paths: testCase.source}
			}

			target.Merge(source)
			assert.Equal(t, target.Paths, testCase.expected)
		})
	}
}

func ptr(str string) *string {
	return &str
}

// Run a benchmark on ReadTerragruntConfig for all fixtures possible.
// This should reveal regressions on execution time due to new, changed or removed features.
func BenchmarkReadTerragruntConfig(b *testing.B) {
	// Setup
	b.StopTimer()
	cwd, err := os.Getwd()
	require.NoError(b, err)

	testDir := "../test"

	fixtureDirs := []struct {
		description          string
		workingDir           string
		usePartialParseCache bool
	}{
		{"PartialParseBenchmarkRegressionCaching", "fixture-regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl", true},
		{"PartialParseBenchmarkRegressionNoCache", "fixture-regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl", false},
		{"PartialParseBenchmarkRegressionIncludesCaching", "fixture-regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl", true},
		{"PartialParseBenchmarkRegressionIncludesNoCache", "fixture-regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl", false},
	}

	// Run benchmarks
	for _, fixture := range fixtureDirs {
		b.Run(fixture.description, func(b *testing.B) {
			workingDir := filepath.Join(cwd, testDir, fixture.workingDir)
			terragruntOptions, err := options.NewTerragruntOptionsForTest(workingDir)
			if fixture.usePartialParseCache {
				terragruntOptions.UsePartialParseConfigCache = true
			} else {
				terragruntOptions.UsePartialParseConfigCache = false
			}
			require.NoError(b, err)

			b.ResetTimer()
			b.StartTimer()
			actual, err := ReadTerragruntConfig(terragruntOptions)
			b.StopTimer()
			require.NoError(b, err)
			require.NotNil(b, actual)
		})
	}
}
