package preprocess

import (
	"bytes"
	"fmt"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestUpdateBackendConfigHappyPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                             string
		inputHcl                         string
		expectedBackendType              string
		expectedBackendConfigHclTemplate string
	}{
		{"S3 backend", s3BackendInput, "s3", s3BackendExpectedOutputTemplate},
		{"S3 backend empty", s3BackendEmptyInput, "s3", s3BackendEmptyExpectedOutputTemplate},
		{"local backend", localBackendInput, "local", localBackendExpectedOutputTemplate},
		{"azure backend", azureBackendInput, "azurerm", azureBackendExpectedOutputTemplate},
		{"consul backend", consulBackendInput, "consul", consulBackendExpectedOutputTemplate},
		{"gcs backend", gcsBackendInput, "gcs", gcsBackendInputExpectedOutputTemplate},
		{"remote backend", remoteBackendInput, "remote", remoteBackendInputExpectedOutputTemplate},
		{"remote backend empty", remoteBackendEmptyInput, "remote", remoteBackendEmptyOutputTemplate},
		{"cloud", cloudBackendInput, "cloud", cloudBackendInputExpectedOutputTemplate},
	}

	for _, testCase := range testCases {
		// capture range variable to avoid it changing across for loop runs during goroutine transitions.
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			block := getParsedHclBlock(t, testCase.inputHcl)

			backend, err := NewTerraformBackend(block)
			require.NoError(t, err)
			require.Equal(t, testCase.expectedBackendType, backend.backendType)

			terragruntOptions, err := options.NewTerragruntOptionsForTest(terragruntConfigPathForTest)
			require.NoError(t, err)

			expectedHcl := fmt.Sprintf(testCase.expectedBackendConfigHclTemplate, envNameForTest, currentModuleNameForTest)

			require.NoError(t, backend.UpdateConfig(currentModuleNameForTest, &envNameForTest, terragruntOptions))
			requireHclEqual(t, expectedHcl, block)
		})
	}
}

func TestUpdateBackendConfigErrorCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                  string
		inputHcl              string
		expectedBackendType   string
		expectedWarningInLogs string
	}{
		{"Unsupported backend - cos", cosBackendInput, "cos", "Backend 'cos' is not yet supported"},
		{"Unsupported backend - http", httpBackendInput, "http", "Backend 'http' is not yet supported"},
		{"Unsupported backend - kubernetes", k8sBackendInput, "kubernetes", "Backend 'kubernetes' is not yet supported"},
		{"Unsupported backend - oss", ossBackendInput, "oss", "Backend 'oss' is not yet supported"},
		{"Unsupported backend - pg", pgBackendInput, "pg", "Backend 'pg' is not yet supported"},
		{"Unsupported workspace config - prefix", remoteBackendWithPrefixInput, "remote", "Terragrunt currently only supports updating the 'name' config in workspaces, but it looks like you're using '[prefix]'."},
		{"Unsupported workspace config - tags", cloudBackendWithTagsInput, "cloud", "Terragrunt currently only supports updating the 'name' config in workspaces, but it looks like you're using '[tags]'."},
	}

	for _, testCase := range testCases {
		// capture range variable to avoid it changing across for loop runs during goroutine transitions.
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			block := getParsedHclBlock(t, testCase.inputHcl)

			backend, err := NewTerraformBackend(block)
			require.NoError(t, err)
			require.Equal(t, testCase.expectedBackendType, backend.backendType)

			terragruntOptions, err := options.NewTerragruntOptionsForTest(terragruntConfigPathForTest)
			require.NoError(t, err)

			// We send warnings to the logs, so capture the log output so we can check it later
			logOutput := new(bytes.Buffer)
			terragruntOptions.Writer = logOutput
			terragruntOptions.ErrWriter = logOutput
			terragruntOptions.Logger = util.CreateLogEntryWithWriter(logOutput, "", logrus.DebugLevel, terragruntOptions.Logger.Logger.Hooks)

			require.NoError(t, backend.UpdateConfig(currentModuleNameForTest, &envNameForTest, terragruntOptions))

			// Ensure the backend config is unchanged
			requireHclEqual(t, testCase.inputHcl, block)

			// Check the log output for the expected warnings
			require.Contains(t, logOutput.String(), testCase.expectedWarningInLogs)
		})
	}
}

func getParsedHclBlock(t *testing.T, hclStr string) *hclwrite.Block {
	parsed, err := hclwrite.ParseConfig([]byte(hclStr), "test.tf", hcl.InitialPos)
	assert.False(t, err.HasErrors(), "Found unexpected error parsing HCL: %s", err.Error())
	require.Len(t, parsed.Body().Blocks(), 1)
	return parsed.Body().Blocks()[0]
}

func requireHclEqual(t *testing.T, expectedHcl string, actualHclBlock *hclwrite.Block) {
	// Convert the block to HCL and format it to give it a consistent syntax
	actualHclAsStr := string(hclwrite.Format(actualHclBlock.BuildTokens(nil).Bytes()))

	// Strip extra leading/trailing whitespace to normalize the values
	expectedHclClean := strings.TrimSpace(expectedHcl)
	actualHclAsStrClean := strings.TrimSpace(actualHclAsStr)

	require.Equal(t, expectedHclClean, actualHclAsStrClean)
}

// Constants used in the tests

const currentModuleNameForTest = "my-test-module"
const otherModuleNameForTest = "my-other-module"

var envNameForTest = "my-env-name"

const terragruntConfigPathForTest = "__not-used__"

const s3BackendInput = `
backend "s3" {
  bucket = "foo"
  region = "us-east-1"
  key    = "terraform.tfstate"
}
`

const s3BackendExpectedOutputTemplate = `
backend "s3" {
  bucket = "foo"
  region = "us-east-1"
  key    = "%s/%s/terraform.tfstate"
}
`

const s3BackendEmptyInput = `
backend "s3" {}
`

const s3BackendEmptyExpectedOutputTemplate = `
backend "s3" {
  key = "%s/%s/terraform.tfstate"
}
`

const localBackendInput = `
backend "local" {
  path = "custom-name.tfstate"
}
`

const localBackendExpectedOutputTemplate = `
backend "local" {
  path = "%s/%s/custom-name.tfstate"
}
`

const azureBackendInput = `
backend "azurerm" {
  resource_group_name  = "StorageAccount-ResourceGroup"
  storage_account_name = "abcd1234"
  container_name       = "tfstate"
  key                  = "prod.terraform.tfstate"
}
`

const azureBackendExpectedOutputTemplate = `
backend "azurerm" {
  resource_group_name  = "StorageAccount-ResourceGroup"
  storage_account_name = "abcd1234"
  container_name       = "tfstate"
  key                  = "%s/%s/prod.terraform.tfstate"
}
`

const consulBackendInput = `
backend "consul" {
  address = "consul.example.com"
  scheme  = "https"
  path    = "full/path"
}
`

const consulBackendExpectedOutputTemplate = `
backend "consul" {
  address = "consul.example.com"
  scheme  = "https"
  path    = "%s/%s/full/path"
}
`

const gcsBackendInput = `
backend "gcs" {
  bucket = "tf-state-prod"
  prefix = "terraform/state"
}
`

const gcsBackendInputExpectedOutputTemplate = `
backend "gcs" {
  bucket = "tf-state-prod"
  prefix = "%s/%s/terraform/state"
}
`

const remoteBackendInput = `
backend "remote" {
  hostname     = "app.terraform.io"
  organization = "company"

  workspaces {
    name = "my-app-prod"
  }
}
`

const remoteBackendInputExpectedOutputTemplate = `
backend "remote" {
  hostname     = "app.terraform.io"
  organization = "company"

  workspaces {
    name = "%s-%s-my-app-prod"
  }
}
`

const remoteBackendEmptyInput = `
backend "remote" {}
`

const remoteBackendEmptyOutputTemplate = `
backend "remote" {
  workspaces {
    name = "%s-%s"
  }
}
`

const cloudBackendInput = `
cloud {
  organization = "my-org"
  hostname     = "app.terraform.io" # Optional; defaults to app.terraform.io

  workspaces {
    name = "vpc"
  }
}
`

const cloudBackendInputExpectedOutputTemplate = `
cloud {
  organization = "my-org"
  hostname     = "app.terraform.io" # Optional; defaults to app.terraform.io

  workspaces {
    name = "%s-%s-vpc"
  }
}
`

const cosBackendInput = `
backend "cos" {
  region = "ap-guangzhou"
  bucket = "bucket-for-terraform-state-1258798060"
  prefix = "terraform/state"
}
`

const k8sBackendInput = `
backend "kubernetes" {
  secret_suffix = "state"
  config_path   = "~/.kube/config"
}
`

const httpBackendInput = `
backend "http" {
  address        = "http://myrest.api.com/foo"
  lock_address   = "http://myrest.api.com/foo"
  unlock_address = "http://myrest.api.com/foo"
}
`

const ossBackendInput = `
backend "oss" {
  bucket              = "bucket-for-terraform-state"
  prefix              = "path/mystate"
  key                 = "version-1.tfstate"
  region              = "cn-beijing"
  tablestore_endpoint = "https://terraform-remote.cn-hangzhou.ots.aliyuncs.com"
  tablestore_table    = "statelock"
}
`

const pgBackendInput = `
backend "pg" {
  conn_str = "postgres://user:pass@db.example.com/terraform_backend"
}
`

const remoteBackendWithPrefixInput = `
backend "remote" {
  hostname     = "app.terraform.io"
  organization = "company"

  workspaces {
    prefix = "my-app-prod"
  }
}
`

const cloudBackendWithTagsInput = `
cloud {
  organization = "my-org"
  hostname     = "app.terraform.io" # Optional; defaults to app.terraform.io

  workspaces {
    tags = ["networking", "apps"]
  }
}
`
