package config_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func createLogger() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))
}

func TestParseTerragruntConfigRemoteStateMinimalConfig(t *testing.T) {
	t.Parallel()

	cfg := `
remote_state {
  backend 	  = "s3"
  config  	  = {}
  encryption  = {}
}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)

	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
		assert.Empty(t, terragruntConfig.RemoteState.BackendConfig)
		assert.Empty(t, terragruntConfig.RemoteState.Encryption)
	}
}

func TestParseTerragruntConfigRemoteStateAttrMinimalConfig(t *testing.T) {
	t.Parallel()

	cfg := `
remote_state = {
  backend = "s3"
  config  = {}
}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)

	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
		assert.Empty(t, terragruntConfig.RemoteState.BackendConfig)
	}
}

func TestParseTerragruntJsonConfigRemoteStateMinimalConfig(t *testing.T) {
	t.Parallel()

	cfg := `
{
	"remote_state": {
		"backend": "s3",
		"config": {}
	}
}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntJSONConfigPath, cfg, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
		assert.Empty(t, terragruntConfig.RemoteState.BackendConfig)
	}
}

func TestParseTerragruntHclConfigRemoteStateMissingBackend(t *testing.T) {
	t.Parallel()

	cfg := `
remote_state {}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	_, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Missing required argument; The argument \"backend\" is required")
}

func TestParseTerragruntHclConfigRemoteStateFullConfig(t *testing.T) {
	t.Parallel()

	cfg := `
remote_state {
	backend = "s3"
	config = {
  		encrypt = true
  		bucket = "my-bucket"
  		key = "terraform.tfstate"
  		region = "us-east-1"
	}
	encryption = {
		key_provider = "pbkdf2"
		passphrase = "correct-horse-battery-staple"
	}
}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
		assert.NotEmpty(t, terragruntConfig.RemoteState.BackendConfig)
		assert.Equal(t, true, terragruntConfig.RemoteState.BackendConfig["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.BackendConfig["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.BackendConfig["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.BackendConfig["region"])
		assert.Equal(t, "pbkdf2", terragruntConfig.RemoteState.Encryption["key_provider"])
		assert.Equal(t, "correct-horse-battery-staple", terragruntConfig.RemoteState.Encryption["passphrase"])
	}
}

func TestParseTerragruntJsonConfigRemoteStateFullConfig(t *testing.T) {
	t.Parallel()

	cfg := `
{
	"remote_state":{
		"backend":"s3",
		"config":{
			"encrypt": true,
			"bucket": "my-bucket",
			"key": "terraform.tfstate",
			"region":"us-east-1"
		},
		"encryption":{
			"key_provider": "pbkdf2",
			"passphrase": "correct-horse-battery-staple"
		}
	}
}
`
	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntJSONConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
		assert.NotEmpty(t, terragruntConfig.RemoteState.BackendConfig)
		assert.Equal(t, true, terragruntConfig.RemoteState.BackendConfig["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.BackendConfig["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.BackendConfig["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.BackendConfig["region"])
		assert.Equal(t, "pbkdf2", terragruntConfig.RemoteState.Encryption["key_provider"])
		assert.Equal(t, "correct-horse-battery-staple", terragruntConfig.RemoteState.Encryption["passphrase"])
	}
}

func TestParseTerragruntHclConfigRetryConfiguration(t *testing.T) {
	t.Parallel()

	// All three legacy retry attributes should be rejected
	cfg := `
retryable_errors = [".*Error.*"]
`
	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	_, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retryable_errors")
	assert.Contains(t, err.Error(), "Unsupported argument")
}

func TestParseTerragruntJsonConfigRetryConfiguration(t *testing.T) {
	t.Parallel()

	cfg := `
{
	"retryable_errors": [".*Error.*"]
}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	_, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntJSONConfigPath, cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retryable_errors")
	// JSON config gives a slightly different error message
	assert.True(t, strings.Contains(err.Error(), "Unsupported argument") || strings.Contains(err.Error(), "No argument or block type"))
}

func TestParseIamRole(t *testing.T) {
	t.Parallel()

	cfg := `iam_role = "terragrunt-iam-role"`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.Dependencies)

	assert.Equal(t, "terragrunt-iam-role", terragruntConfig.IamRole)
}

func TestParseIamAssumeRoleDuration(t *testing.T) {
	t.Parallel()

	cfg := `iam_assume_role_duration = 36000`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.Dependencies)

	assert.Equal(t, int64(36000), *terragruntConfig.IamAssumeRoleDuration)
}

func TestParseIamAssumeRoleSessionName(t *testing.T) {
	t.Parallel()

	cfg := `iam_assume_role_session_name = "terragrunt-iam-assume-role-session-name"`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.Dependencies)

	assert.Equal(t, "terragrunt-iam-assume-role-session-name", terragruntConfig.IamAssumeRoleSessionName)
}

func TestParseIamWebIdentity(t *testing.T) {
	t.Parallel()

	token := "test-token"

	cfg := fmt.Sprintf(`iam_web_identity_token = "%s"`, token)

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Empty(t, terragruntConfig.IamRole)
	assert.Equal(t, token, terragruntConfig.IamWebIdentityToken)
}

func TestParseTerragruntConfigDependenciesOnePath(t *testing.T) {
	t.Parallel()

	cfg := `
dependencies {
	paths = ["../test/fixtures/parent-folders/multiple-terragrunt-in-parents"]
}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)

	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixtures/parent-folders/multiple-terragrunt-in-parents"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigDependenciesMultiplePaths(t *testing.T) {
	t.Parallel()

	cfg := `
dependencies {
	paths = ["../test/fixtures/terragrunt", "../test/fixtures/dirs", "../test/fixtures/inputs"]
}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixtures/terragrunt", "../test/fixtures/dirs", "../test/fixtures/inputs"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigRemoteStateDynamoDbTerraformConfigAndDependenciesFullConfig(t *testing.T) {
	t.Parallel()

	cfg := `
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
	paths = ["../test/fixtures/terragrunt", "../test/fixtures/dirs", "../test/fixtures/inputs"]
}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, terragruntConfig.Terraform)
	assert.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
		assert.NotEmpty(t, terragruntConfig.RemoteState.BackendConfig)
		assert.Equal(t, true, terragruntConfig.RemoteState.BackendConfig["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.BackendConfig["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.BackendConfig["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.BackendConfig["region"])
	}

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixtures/terragrunt", "../test/fixtures/dirs", "../test/fixtures/inputs"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntJsonConfigRemoteStateDynamoDbTerraformConfigAndDependenciesFullConfig(t *testing.T) {
	t.Parallel()

	cfg := `
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
		"paths": ["../test/fixtures/terragrunt", "../test/fixtures/dirs", "../test/fixtures/inputs"]
	}
}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntJSONConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, terragruntConfig.Terraform)
	assert.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)
	assert.Empty(t, terragruntConfig.IamRole)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
		assert.NotEmpty(t, terragruntConfig.RemoteState.BackendConfig)
		assert.Equal(t, true, terragruntConfig.RemoteState.BackendConfig["encrypt"])
		assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.BackendConfig["bucket"])
		assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.BackendConfig["key"])
		assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.BackendConfig["region"])
	}

	if assert.NotNil(t, terragruntConfig.Dependencies) {
		assert.Equal(t, []string{"../test/fixtures/terragrunt", "../test/fixtures/dirs", "../test/fixtures/inputs"}, terragruntConfig.Dependencies.Paths)
	}
}

func TestParseTerragruntConfigInclude(t *testing.T) {
	t.Parallel()

	cfg :=
		fmt.Sprintf(`
include {
	path = "../../../%s"
}
`, "root.hcl")

	opts := &options.TerragruntOptions{
		TerragruntConfigPath: "../test/fixtures/parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/" + config.DefaultTerragruntConfigPath,
		NonInteractive:       true,
		StrictControls:       controls.New(),
	}

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, opts)

	terragruntConfig, err := config.ParseConfigString(ctx, l, opts.TerragruntConfigPath, cfg, nil)
	if assert.NoError(t, err, "Unexpected error: %v", errors.New(err)) {
		assert.Nil(t, terragruntConfig.Terraform)

		if assert.NotNil(t, terragruntConfig.RemoteState) {
			assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
			assert.NotEmpty(t, terragruntConfig.RemoteState.BackendConfig)
			assert.Equal(t, true, terragruntConfig.RemoteState.BackendConfig["encrypt"])
			assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.BackendConfig["bucket"])
			assert.Equal(t, "child/sub-child/sub-sub-child/terraform.tfstate", terragruntConfig.RemoteState.BackendConfig["key"])
			assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.BackendConfig["region"])
		}
	}
}

func TestParseTerragruntConfigIncludeWithFindInParentFolders(t *testing.T) {
	t.Parallel()

	cfg := `
include {
	path = find_in_parent_folders("root.hcl")
}
`

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixtures/parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath)

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, opts)

	terragruntConfig, err := config.ParseConfigString(ctx, l, opts.TerragruntConfigPath, cfg, nil)
	if assert.NoError(t, err, "Unexpected error: %v", errors.New(err)) {
		assert.Nil(t, terragruntConfig.Terraform)

		if assert.NotNil(t, terragruntConfig.RemoteState) {
			assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
			assert.NotEmpty(t, terragruntConfig.RemoteState.BackendConfig)
			assert.Equal(t, true, terragruntConfig.RemoteState.BackendConfig["encrypt"])
			assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.BackendConfig["bucket"])
			assert.Equal(t, "child/sub-child/sub-sub-child/terraform.tfstate", terragruntConfig.RemoteState.BackendConfig["key"])
			assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.BackendConfig["region"])
		}
	}
}

func TestParseTerragruntConfigIncludeOverrideRemote(t *testing.T) {
	t.Parallel()

	cfg :=
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
`, "root.hcl")

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixtures/parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath)

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, opts)

	terragruntConfig, err := config.ParseConfigString(ctx, l, opts.TerragruntConfigPath, cfg, nil)
	if assert.NoError(t, err, "Unexpected error: %v", errors.New(err)) {
		assert.Nil(t, terragruntConfig.Terraform)

		if assert.NotNil(t, terragruntConfig.RemoteState) {
			assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
			assert.NotEmpty(t, terragruntConfig.RemoteState.BackendConfig)
			assert.Equal(t, false, terragruntConfig.RemoteState.BackendConfig["encrypt"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.BackendConfig["bucket"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.BackendConfig["key"])
			assert.Equal(t, "override", terragruntConfig.RemoteState.BackendConfig["region"])
		}
	}
}

func TestParseTerragruntConfigIncludeOverrideAll(t *testing.T) {
	t.Parallel()

	cfg :=
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
`, "root.hcl")

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixtures/parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+config.DefaultTerragruntConfigPath)

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, opts)
	terragruntConfig, err := config.ParseConfigString(ctx, l, opts.TerragruntConfigPath, cfg, nil)
	require.NoError(t, err, "Unexpected error: %v", errors.New(err))

	assert.NotNil(t, terragruntConfig.Terraform)
	assert.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
		assert.NotEmpty(t, terragruntConfig.RemoteState.BackendConfig)
		assert.Equal(t, false, terragruntConfig.RemoteState.BackendConfig["encrypt"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.BackendConfig["bucket"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.BackendConfig["key"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.BackendConfig["region"])
	}

	assert.Equal(t, []string{"override"}, terragruntConfig.Dependencies.Paths)
}

func TestParseTerragruntJsonConfigIncludeOverrideAll(t *testing.T) {
	t.Parallel()

	cfg :=
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
`, "root.hcl")

	opts := mockOptionsForTestWithConfigPath(t, "../test/fixtures/parent-folders/terragrunt-in-root/child/sub-child/sub-sub-child/"+config.DefaultTerragruntJSONConfigPath)

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, opts)
	terragruntConfig, err := config.ParseConfigString(ctx, l, opts.TerragruntConfigPath, cfg, nil)
	require.NoError(t, err, "Unexpected error: %v", errors.New(err))

	assert.NotNil(t, terragruntConfig.Terraform)
	assert.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)

	if assert.NotNil(t, terragruntConfig.RemoteState) {
		assert.Equal(t, "s3", terragruntConfig.RemoteState.BackendName)
		assert.NotEmpty(t, terragruntConfig.RemoteState.BackendConfig)
		assert.Equal(t, false, terragruntConfig.RemoteState.BackendConfig["encrypt"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.BackendConfig["bucket"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.BackendConfig["key"])
		assert.Equal(t, "override", terragruntConfig.RemoteState.BackendConfig["region"])
	}

	assert.Equal(t, []string{"override"}, terragruntConfig.Dependencies.Paths)
}

func TestParseTerragruntConfigTwoLevels(t *testing.T) {
	t.Parallel()

	configPath := "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/sub-child/" + config.RecommendedParentConfigName

	cfg, err := util.ReadFileAsString(configPath)
	if err != nil {
		t.Fatal(err)
	}

	opts := mockOptionsForTestWithConfigPath(t, configPath)
	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, opts)

	_, actualErr := config.ParseConfigString(ctx, l, configPath, cfg, nil)

	errStr := actualErr.Error()

	expectedErrPath := filepath.ToSlash(absPath(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/"+config.RecommendedParentConfigName))
	expectedErrStr := fmt.Sprintf("%s includes %s, which itself includes %s. Only one level of includes is allowed.",
		configPath, expectedErrPath, expectedErrPath)

	assert.Contains(t, errStr, expectedErrStr)
}

func TestParseTerragruntConfigThreeLevels(t *testing.T) {
	t.Parallel()

	configPath := "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/sub-child/sub-sub-child/" + config.DefaultTerragruntConfigPath

	cfg, err := util.ReadFileAsString(configPath)
	if err != nil {
		t.Fatal(err)
	}

	opts := mockOptionsForTestWithConfigPath(t, configPath)
	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, opts)

	_, actualErr := config.ParseConfigString(ctx, l, configPath, cfg, nil)

	// Convert the error paths to forward slashes for cross-platform compatibility
	errStr := actualErr.Error()
	errStr = strings.ReplaceAll(errStr, "\\", "/")

	// Build expected error string
	expectedErrPath1 := filepath.ToSlash(absPath(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+config.RecommendedParentConfigName))
	expectedErrPath2 := filepath.ToSlash(absPath(t, "../test/fixtures/parent-folders/multiple-terragrunt-in-parents/child/sub-child/"+config.RecommendedParentConfigName))
	expectedErrStr := fmt.Sprintf("%s includes %s, which itself includes %s. Only one level of includes is allowed.",
		configPath, expectedErrPath1, expectedErrPath2)

	assert.Contains(t, errStr, expectedErrStr)
}

func TestParseTerragruntConfigEmptyConfig(t *testing.T) {
	t.Parallel()

	cfg := ``

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Empty(t, terragruntConfig.IamRole)
	assert.Empty(t, terragruntConfig.IamWebIdentityToken)
}

func TestParseTerragruntConfigEmptyConfigOldConfig(t *testing.T) {
	t.Parallel()

	cfgString := ``

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	cfg, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfgString, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, cfg.RemoteState)
}

func TestParseTerragruntConfigTerraformNoSource(t *testing.T) {
	t.Parallel()

	cfg := `
terraform {}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	assert.NotNil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.Terraform.Source)
}

func TestParseTerragruntConfigTerraformWithSource(t *testing.T) {
	t.Parallel()

	cfg := `
terraform {
	source = "foo"
}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)

	assert.NotNil(t, terragruntConfig.Terraform)
	assert.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "foo", *terragruntConfig.Terraform.Source)
}

func TestParseTerragruntConfigTerraformWithExtraArguments(t *testing.T) {
	t.Parallel()

	cfg := `
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

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
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
			config.TerraformCommandsNeedVars,
			terragruntConfig.Terraform.ExtraArgs[0].Commands)

		assert.Equal(t,
			&map[string]string{"TEST_VAR": "value"},
			terragruntConfig.Terraform.ExtraArgs[0].EnvVars)
	}
}

func TestParseTerragruntConfigTerraformWithMultipleExtraArguments(t *testing.T) {
	t.Parallel()

	cfg := `
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

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
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
		assert.Equal(t, config.TerraformCommandsNeedVars, terragruntConfig.Terraform.ExtraArgs[2].Commands)
		assert.Equal(t, "optional_tfvars", terragruntConfig.Terraform.ExtraArgs[3].Name)
		assert.Equal(t, &[]string{"opt1.tfvars", "opt2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[3].OptionalVarFiles)
		assert.Equal(t, config.TerraformCommandsNeedVars, terragruntConfig.Terraform.ExtraArgs[3].Commands)
	}
}

func TestParseTerragruntJsonConfigTerraformWithMultipleExtraArguments(t *testing.T) {
	t.Parallel()

	cfg := `
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

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntJSONConfigPath, cfg, nil)
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
		assert.Equal(t, config.TerraformCommandsNeedVars, terragruntConfig.Terraform.ExtraArgs[2].Commands)
		assert.Equal(t, "optional_tfvars", terragruntConfig.Terraform.ExtraArgs[3].Name)
		assert.Equal(t, &[]string{"opt1.tfvars", "opt2.tfvars"}, terragruntConfig.Terraform.ExtraArgs[3].OptionalVarFiles)
		assert.Equal(t, config.TerraformCommandsNeedVars, terragruntConfig.Terraform.ExtraArgs[3].Commands)
	}
}

func TestFindConfigFilesInPathNone(t *testing.T) {
	t.Parallel()

	expected := []string{}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := config.FindConfigFilesInPath("../test/fixtures/config-files/none", terragruntOptions)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathOneConfig(t *testing.T) {
	t.Parallel()

	expected := []string{"../test/fixtures/config-files/one-config/subdir/terragrunt.hcl"}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := config.FindConfigFilesInPath("../test/fixtures/config-files/one-config", terragruntOptions)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathOneJsonConfig(t *testing.T) {
	t.Parallel()

	expected := []string{"../test/fixtures/config-files/one-json-config/subdir/terragrunt.hcl.json"}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := config.FindConfigFilesInPath("../test/fixtures/config-files/one-json-config", terragruntOptions)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesInPathMultipleConfigs(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixtures/config-files/multiple-configs/terragrunt.hcl",
		"../test/fixtures/config-files/multiple-configs/subdir-2/subdir/terragrunt.hcl",
		"../test/fixtures/config-files/multiple-configs/subdir-3/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := config.FindConfigFilesInPath("../test/fixtures/config-files/multiple-configs", terragruntOptions)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesInPathMultipleJsonConfigs(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixtures/config-files/multiple-json-configs/terragrunt.hcl.json",
		"../test/fixtures/config-files/multiple-json-configs/subdir-2/subdir/terragrunt.hcl.json",
		"../test/fixtures/config-files/multiple-json-configs/subdir-3/terragrunt.hcl.json",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := config.FindConfigFilesInPath("../test/fixtures/config-files/multiple-json-configs", terragruntOptions)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesInPathMultipleMixedConfigs(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixtures/config-files/multiple-mixed-configs/terragrunt.hcl.json",
		"../test/fixtures/config-files/multiple-mixed-configs/subdir-2/subdir/terragrunt.hcl",
		"../test/fixtures/config-files/multiple-mixed-configs/subdir-3/terragrunt.hcl.json",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := config.FindConfigFilesInPath("../test/fixtures/config-files/multiple-mixed-configs", terragruntOptions)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerragruntCache(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixtures/config-files/ignore-cached-config/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := config.FindConfigFilesInPath("../test/fixtures/config-files/ignore-cached-config", terragruntOptions)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.Equal(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDir(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixtures/config-files/ignore-terraform-data-dir/.tf_data/modules/mod/terragrunt.hcl",
		"../test/fixtures/config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl",
		"../test/fixtures/config-files/ignore-terraform-data-dir/subdir/.tf_data/modules/mod/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	actual, err := config.FindConfigFilesInPath("../test/fixtures/config-files/ignore-terraform-data-dir", terragruntOptions)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDirEnv(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixtures/config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl",
		"../test/fixtures/config-files/ignore-terraform-data-dir/subdir/.terraform/modules/mod/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	terragruntOptions.Env["TF_DATA_DIR"] = ".tf_data"

	actual, err := config.FindConfigFilesInPath("../test/fixtures/config-files/ignore-terraform-data-dir", terragruntOptions)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDirEnvPath(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixtures/config-files/ignore-terraform-data-dir/.tf_data/modules/mod/terragrunt.hcl",
		"../test/fixtures/config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl",
		"../test/fixtures/config-files/ignore-terraform-data-dir/subdir/.terraform/modules/mod/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	terragruntOptions.Env["TF_DATA_DIR"] = "subdir/.tf_data"

	actual, err := config.FindConfigFilesInPath("../test/fixtures/config-files/ignore-terraform-data-dir", terragruntOptions)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func TestFindConfigFilesIgnoresTerraformDataDirEnvRoot(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	require.NoError(t, err)

	workingDir := filepath.Join(cwd, "../test/fixtures/config-files/ignore-terraform-data-dir/")
	terragruntOptions, err := options.NewTerragruntOptionsForTest(workingDir)
	require.NoError(t, err)

	terragruntOptions.Env["TF_DATA_DIR"] = filepath.Join(workingDir, ".tf_data")

	actual, err := config.FindConfigFilesInPath(workingDir, terragruntOptions)
	require.NoError(t, err, "Unexpected error: %v", err)

	// Create expected paths using filepath.Join for cross-platform compatibility
	expected := []string{
		filepath.Join(cwd, "../test/fixtures/config-files/ignore-terraform-data-dir/subdir/terragrunt.hcl"),
		filepath.Join(cwd, "../test/fixtures/config-files/ignore-terraform-data-dir/subdir/.terraform/modules/mod/terragrunt.hcl"),
		filepath.Join(cwd, "../test/fixtures/config-files/ignore-terraform-data-dir/subdir/.tf_data/modules/mod/terragrunt.hcl"),
	}

	// Sort both slices to ensure consistent order for comparison
	sort.Strings(actual)
	sort.Strings(expected)

	// Compare the paths using filepath.Clean to normalize them
	normalizedActual := make([]string, len(actual))
	normalizedExpected := make([]string, len(expected))

	for i, path := range actual {
		normalizedActual[i] = filepath.Clean(path)
	}

	for i, path := range expected {
		normalizedExpected[i] = filepath.Clean(path)
	}

	assert.Equal(t, normalizedExpected, normalizedActual)
}

func TestFindConfigFilesIgnoresDownloadDir(t *testing.T) {
	t.Parallel()

	expected := []string{
		"../test/fixtures/config-files/multiple-configs/terragrunt.hcl",
		"../test/fixtures/config-files/multiple-configs/subdir-3/terragrunt.hcl",
	}
	terragruntOptions, err := options.NewTerragruntOptionsForTest("test")
	require.NoError(t, err)

	terragruntOptions.DownloadDir = "../test/fixtures/config-files/multiple-configs/subdir-2"

	actual, err := config.FindConfigFilesInPath("../test/fixtures/config-files/multiple-configs", terragruntOptions)

	require.NoError(t, err, "Unexpected error: %v", err)
	assert.ElementsMatch(t, expected, actual)
}

func mockOptionsForTestWithConfigPath(t *testing.T, configPath string) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(configPath)
	if err != nil {
		t.Fatalf("Failed to create TerragruntOptions: %v", err)
	}

	return opts
}

func mockOptionsForTest(t *testing.T) *options.TerragruntOptions {
	t.Helper()

	return mockOptionsForTestWithConfigPath(t, "test-time-mock")
}

func TestParseTerragruntConfigPreventDestroyTrue(t *testing.T) {
	t.Parallel()

	cfg := `
prevent_destroy = true
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.True(t, *terragruntConfig.PreventDestroy)
}

func TestParseTerragruntConfigPreventDestroyFalse(t *testing.T) {
	t.Parallel()

	cfg := `
prevent_destroy = false
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.False(t, *terragruntConfig.PreventDestroy)
}

func TestParseTerragruntConfigSkipTrue(t *testing.T) {
	t.Parallel()

	cfg := `
skip = true
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	_, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skip")
	assert.Contains(t, err.Error(), "Unsupported argument")
}

func TestParseTerragruntConfigSkipFalse(t *testing.T) {
	t.Parallel()

	cfg := `
skip = false
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	_, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skip")
	assert.Contains(t, err.Error(), "Unsupported argument")
}

func TestIncludeFunctionsWorkInChildConfig(t *testing.T) {
	t.Parallel()

	cfg := `
include {
	path = find_in_parent_folders("root.hcl")
}
terraform {
	source = path_relative_to_include()
}
`
	opts := &options.TerragruntOptions{
		TerragruntConfigPath: "../test/fixtures/parent-folders/terragrunt-in-root/child/" + config.DefaultTerragruntConfigPath,
		NonInteractive:       true,
		MaxFoldersToCheck:    5,
		StrictControls:       controls.New(),
	}

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, opts)

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "child", *terragruntConfig.Terraform.Source)
}

func TestModuleDependenciesMerge(t *testing.T) {
	t.Parallel()

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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			target := &config.ModuleDependencies{Paths: tc.target}

			var source *config.ModuleDependencies = nil
			if tc.source != nil {
				source = &config.ModuleDependencies{Paths: tc.source}
			}

			target.Merge(source)
			assert.Equal(t, tc.expected, target.Paths)
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
		{"PartialParseBenchmarkRegressionCaching", "regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl", true},
		{"PartialParseBenchmarkRegressionNoCache", "regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl", false},
		{"PartialParseBenchmarkRegressionIncludesCaching", "regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl", true},
		{"PartialParseBenchmarkRegressionIncludesNoCache", "regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl", false},
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

			l := createLogger()

			b.ResetTimer()
			b.StartTimer()
			actual, err := config.ReadTerragruntConfig(b.Context(), l, terragruntOptions, config.DefaultParserOptions(l, terragruntOptions))
			b.StopTimer()
			require.NoError(b, err)
			assert.NotNil(b, actual)
		})
	}
}

func TestBestEffortParseConfigString(t *testing.T) {
	t.Parallel()

	tc := []struct {
		expectedConfig *config.TerragruntConfig
		name           string
		cfg            string
		expectError    bool
	}{
		{
			name: "Simple",
			cfg: `locals {
	simple        = "value"
	requires_auth = run_cmd("exit", "1") // intentional error
}
`,
			expectError: true,
			expectedConfig: &config.TerragruntConfig{
				Locals: map[string]any{
					"simple": "value",
				},
				GenerateConfigs:   map[string]codegen.GenerateConfig{},
				ProcessedIncludes: config.IncludeConfigsMap{},
				FieldsMetadata: map[string]map[string]any{
					"locals-simple": {
						"found_in_file": "terragrunt.hcl",
					},
				},
			},
		},
		{
			name: "Locals referencing each other",
			cfg: `locals {
	reference = local.simple
	simple    = "value"
}
`,
			expectError: false,
			expectedConfig: &config.TerragruntConfig{
				Locals: map[string]any{
					"reference": "value",
					"simple":    "value",
				},
				GenerateConfigs:   map[string]codegen.GenerateConfig{},
				ProcessedIncludes: config.IncludeConfigsMap{},
				FieldsMetadata: map[string]map[string]any{
					"locals-reference": {
						"found_in_file": "terragrunt.hcl",
					},
					"locals-simple": {
						"found_in_file": "terragrunt.hcl",
					},
				},
			},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := createLogger()

			ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

			terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, tt.cfg, nil)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.expectedConfig, terragruntConfig)
		})
	}
}

func TestParseConfigWithMissingIfExists(t *testing.T) {
	t.Parallel()

	cfg := `generate "test" {
  path     = "test.tf"
  contents = "foo"
}`

	l := createLogger()
	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.Error(t, err)

	errStr := err.Error()
	hasIfExistsError := strings.Contains(errStr, "if_exists")
	hasGenerateError := strings.Contains(errStr, "generate") || strings.Contains(errStr, "Missing required argument")
	assert.True(t, hasIfExistsError || hasGenerateError, "Error message should mention missing if_exists attribute or generate block. Got: %s", errStr)
	assert.NotNil(t, terragruntConfig)
}

func TestBestEffortParseConfigStringWDependency(t *testing.T) {
	t.Parallel()

	depCfg := `locals {
	simple = "value"
	fail   = run_cmd("exit", "1") // intentional error
}`

	cfg := `locals {
	simple = "value"
	fail   = run_cmd("exit", "1") // intentional error
}

dependency "dep" {
	config_path = "../dep"
}`

	tmpDir := t.TempDir()

	depPath := filepath.Join(tmpDir, "dep")
	require.NoError(t, os.MkdirAll(depPath, 0755))

	depCfgPath := filepath.Join(depPath, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(depCfgPath, []byte(depCfg), 0644))

	unitPath := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitPath, 0755))

	unitCfgPath := filepath.Join(unitPath, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(unitCfgPath, []byte(cfg), 0644))

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))

	ctx.TerragruntOptions.WorkingDir = unitPath

	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.Error(t, err)

	assert.Equal(t, &config.TerragruntConfig{
		Locals: map[string]any{
			"simple": "value",
		},
		GenerateConfigs:   map[string]codegen.GenerateConfig{},
		ProcessedIncludes: config.IncludeConfigsMap{},
		FieldsMetadata: map[string]map[string]any{
			"dependency-dep": {
				"found_in_file": "terragrunt.hcl",
			},
			"locals-simple": {
				"found_in_file": "terragrunt.hcl",
			},
		},
		TerragruntDependencies: config.Dependencies{
			config.Dependency{
				Name:       "dep",
				ConfigPath: cty.StringVal("../dep"),
			},
		},
	}, terragruntConfig)
}

func TestWriteTo(t *testing.T) {
	t.Parallel()

	cfg := `
locals {
	string = "value"
	bool   = true
	number = 123
	list   = ["a", "b", "c"]
}

terraform {
	source = "git::git@github.com:org/repo.git//modules/test?ref=v0.1.0"

	extra_arguments "secrets" {
		commands = ["plan", "apply"]
		arguments = ["-var-file=secrets.tfvars"]
		required_var_files = ["common.tfvars"]
		optional_var_files = ["optional.tfvars"]
		env_vars = {
			TEST_VAR = "value"
		}
	}

	before_hook "before" {
		commands = ["plan", "apply"]
		execute  = ["echo", "before"]
		working_dir = "before_dir"
	}

	after_hook "after" {
		commands = ["plan", "apply"]
		execute  = ["echo", "after"]
		working_dir = "after_dir"
	}

	error_hook "error" {
		commands = ["plan", "apply"]
		execute  = ["echo", "error"]
		on_errors = [
			".*Error.*",
			".*Exception.*"
		]
		working_dir = "error_dir"
	}
}

engine {
	source = "github.com/gruntwork-io/terragrunt"
	version = "v0.1.0"
	type = "rpc"
	meta = {
		key = "value"
	}
}

exclude {
	exclude_dependencies = true
	actions = ["init", "plan"]
	if = true
}

errors {
	retry "test_retry" {
		max_attempts = 3
		sleep_interval_sec = 5
		retryable_errors = [
			".*Error.*",
			".*Exception.*"
		]
	}

	ignore "test_ignore" {
		ignorable_errors = [
			".*Warning.*",
			".*Deprecated.*"
		]
		message = "Ignoring warning messages"
		signals = {
			key = "value"
		}
	}
}

// The catalog block won't actually show up when using
// ParseConfigString. It probably should, but that's not
// a problem for this test.
//
// catalog {
// 	default_template = "default.hcl"
// 	urls = [
// 		"github.com/org/repo//templates/template1.hcl",
// 		"github.com/org/repo//templates/template2.hcl"
// 	]
// }

remote_state {
	backend = "s3"
	disable_init = true
	disable_dependency_optimization = true
	config = {
		bucket = "my-bucket"
		key    = "terraform.tfstate"
		region = "us-east-1"
	}
}

// These aren't worth testing because they require filesystem operations
// as currently implemented, and we don't want to do that in this test.
//
// dependencies {
// 	paths = ["../vpc", "../database"]
// }

// dependency "vpc" {
// 	config_path = "../vpc"
// 	skip_outputs = true
// 	mock_outputs = {
// 		vpc_id = "mock-vpc-id"
// 	}
// }

generate "provider" {
	path = "provider.tf"
	if_exists = "overwrite"
	contents = <<EOF
provider "aws" {
	region = "us-east-1"
}
EOF
	comment_prefix = "//"
	disable_signature = true
	disable = false
}

feature "test_feature" {
	default = true
}

terraform_binary = "terraform"
terraform_version_constraint = ">= 1.0.0"
terragrunt_version_constraint = ">= 0.36.0"
download_dir = ".terragrunt-cache"
prevent_destroy = true
iam_role = "arn:aws:iam::ACCOUNT_ID:role/ROLE_NAME"
iam_assume_role_duration = 3600
iam_assume_role_session_name = "terragrunt"

inputs = {
	string = "value"
	bool   = true
	number = 123
	list   = ["a", "b", "c"]
	map    = {
		key = "value"
	}
}
`

	l := createLogger()

	ctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	terragruntConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)

	// Write the config to a buffer
	buf := &bytes.Buffer{}
	n, err := terragruntConfig.WriteTo(buf)
	require.NoError(t, err)
	assert.Positive(t, n)

	// Parse the written config back
	rereadConfig, err := config.ParseConfigString(ctx, l, config.DefaultTerragruntConfigPath, buf.String(), nil)
	require.NoError(t, err)

	// Verify the configs match
	assert.Equal(t, terragruntConfig.Locals, rereadConfig.Locals)
	assert.Equal(t, terragruntConfig.Terraform.Source, rereadConfig.Terraform.Source)
	assert.Equal(t, terragruntConfig.Terraform.ExtraArgs, rereadConfig.Terraform.ExtraArgs)
	assert.Equal(t, terragruntConfig.Terraform.BeforeHooks, rereadConfig.Terraform.BeforeHooks)
	assert.Equal(t, terragruntConfig.Terraform.AfterHooks, rereadConfig.Terraform.AfterHooks)
	assert.Equal(t, terragruntConfig.Terraform.ErrorHooks, rereadConfig.Terraform.ErrorHooks)

	// Test engine block
	assert.Equal(t, terragruntConfig.Engine.Source, rereadConfig.Engine.Source)
	assert.Equal(t, terragruntConfig.Engine.Version, rereadConfig.Engine.Version)
	assert.Equal(t, terragruntConfig.Engine.Type, rereadConfig.Engine.Type)
	assert.Equal(t, terragruntConfig.Engine.Meta, rereadConfig.Engine.Meta)

	// Test exclude block
	assert.Equal(t, terragruntConfig.Exclude.ExcludeDependencies, rereadConfig.Exclude.ExcludeDependencies)
	assert.Equal(t, terragruntConfig.Exclude.Actions, rereadConfig.Exclude.Actions)
	assert.Equal(t, terragruntConfig.Exclude.If, rereadConfig.Exclude.If)

	// Test errors block
	assert.Len(t, terragruntConfig.Errors.Retry, len(rereadConfig.Errors.Retry))

	if len(terragruntConfig.Errors.Retry) > 0 {
		assert.Equal(t, terragruntConfig.Errors.Retry[0].Label, rereadConfig.Errors.Retry[0].Label)
		assert.Equal(t, terragruntConfig.Errors.Retry[0].MaxAttempts, rereadConfig.Errors.Retry[0].MaxAttempts)
		assert.Equal(t, terragruntConfig.Errors.Retry[0].SleepIntervalSec, rereadConfig.Errors.Retry[0].SleepIntervalSec)
		assert.Equal(t, terragruntConfig.Errors.Retry[0].RetryableErrors, rereadConfig.Errors.Retry[0].RetryableErrors)
	}

	assert.Len(t, terragruntConfig.Errors.Ignore, len(rereadConfig.Errors.Ignore))

	if len(terragruntConfig.Errors.Ignore) > 0 {
		assert.Equal(t, terragruntConfig.Errors.Ignore[0].Label, rereadConfig.Errors.Ignore[0].Label)
		assert.Equal(t, terragruntConfig.Errors.Ignore[0].IgnorableErrors, rereadConfig.Errors.Ignore[0].IgnorableErrors)
		assert.Equal(t, terragruntConfig.Errors.Ignore[0].Message, rereadConfig.Errors.Ignore[0].Message)
		assert.Equal(t, terragruntConfig.Errors.Ignore[0].Signals, rereadConfig.Errors.Ignore[0].Signals)
	}

	// The catalog block won't actually show up when using
	// ParseConfigString. It probably should, but that's not
	// a problem for this test.
	//
	// assert.Equal(t, terragruntConfig.Catalog.DefaultTemplate, rereadConfig.Catalog.DefaultTemplate)
	// assert.Equal(t, terragruntConfig.Catalog.URLs, rereadConfig.Catalog.URLs)

	assert.Equal(t, terragruntConfig.RemoteState.BackendName, rereadConfig.RemoteState.BackendName)
	assert.Equal(t, terragruntConfig.RemoteState.DisableInit, rereadConfig.RemoteState.DisableInit)
	assert.Equal(t, terragruntConfig.RemoteState.DisableDependencyOptimization, rereadConfig.RemoteState.DisableDependencyOptimization)
	assert.Equal(t, terragruntConfig.RemoteState.BackendConfig, rereadConfig.RemoteState.BackendConfig)

	// We don't test dependencies here because they require filesystem operations.
	// assert.Equal(t, terragruntConfig.Dependencies.Paths, rereadConfig.Dependencies.Paths)
	// assert.Equal(t, terragruntConfig.TerragruntDependencies, rereadConfig.TerragruntDependencies)

	assert.Equal(t, terragruntConfig.GenerateConfigs, rereadConfig.GenerateConfigs)
	assert.Equal(t, terragruntConfig.FeatureFlags, rereadConfig.FeatureFlags)
	assert.Equal(t, terragruntConfig.TerraformBinary, rereadConfig.TerraformBinary)
	assert.Equal(t, terragruntConfig.TerraformVersionConstraint, rereadConfig.TerraformVersionConstraint)
	assert.Equal(t, terragruntConfig.TerragruntVersionConstraint, rereadConfig.TerragruntVersionConstraint)
	assert.Equal(t, terragruntConfig.DownloadDir, rereadConfig.DownloadDir)
	assert.Equal(t, terragruntConfig.PreventDestroy, rereadConfig.PreventDestroy)
	assert.Equal(t, terragruntConfig.IamRole, rereadConfig.IamRole)
	assert.Equal(t, terragruntConfig.IamAssumeRoleDuration, rereadConfig.IamAssumeRoleDuration)
	assert.Equal(t, terragruntConfig.IamAssumeRoleSessionName, rereadConfig.IamAssumeRoleSessionName)
	assert.Equal(t, terragruntConfig.Inputs, rereadConfig.Inputs)
}
