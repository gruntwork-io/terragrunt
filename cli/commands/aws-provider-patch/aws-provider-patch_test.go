package awsproviderpatch_test

import (
	"testing"

	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const terraformCodeExampleOutputOnly = `
output "hello" {
  value = "Hello, World"
}
`

const terraformCodeExampleGcpProvider = `
provider "google" {
  credentials = file("account.json")
  project     = "my-project-id"
  region      = "us-central1"
}

output "hello" {
  value = "Hello, World"
}
`

const terraformCodeExampleAwsProviderEmptyOriginal = `
provider "aws" {
}

output "hello" {
  value = "Hello, World"
}
`

const terraformCodeExampleAwsProviderRegionVersionOverridenExpected = `
provider "aws" {
  region  = "eu-west-1"
  version = "0.3.0"
}

output "hello" {
  value = "Hello, World"
}
`

const terraformCodeExampleAwsProviderRegionVersionOverridenReverseOrderExpected = `
provider "aws" {
  version = "0.3.0"
  region  = "eu-west-1"
}

output "hello" {
  value = "Hello, World"
}
`

const terraformCodeExampleAwsProviderNonEmptyOriginal = `
provider "aws" {
  region  = var.aws_region
  version = "0.2.0"
}

output "hello" {
  value = "Hello, World"
}
`

const terraformCodeExampleAwsProviderRegionOverridenVersionNotOverriddenExpected = `
provider "aws" {
  region  = "eu-west-1"
  version = "0.2.0"
}

output "hello" {
  value = "Hello, World"
}
`

const terraformCodeExampleAwsMultipleProvidersOriginal = `
provider "aws" {
  region  = var.aws_region
  version = "0.2.0"
}

provider "aws" {
  alias   = "another"
  region  = var.aws_region
  version = "0.2.0"
}

resource "aws_instance" "example" {

}

provider "google" {
  credentials = file("account.json")
  project     = "my-project-id"
  region      = "us-central1"
}

provider "aws" {
  alias  = "yet another"
  region = var.aws_region
}

output "hello" {
  value = "Hello, World"
}
`

const terraformCodeExampleAwsMultipleProvidersRegionOverridenExpected = `
provider "aws" {
  region  = "eu-west-1"
  version = "0.2.0"
}

provider "aws" {
  alias   = "another"
  region  = "eu-west-1"
  version = "0.2.0"
}

resource "aws_instance" "example" {

}

provider "google" {
  credentials = file("account.json")
  project     = "my-project-id"
  region      = "us-central1"
}

provider "aws" {
  alias  = "yet another"
  region = "eu-west-1"
}

output "hello" {
  value = "Hello, World"
}
`

const terraformCodeExampleAwsMultipleProvidersRegionVersionOverridenExpected = `
provider "aws" {
  region  = "eu-west-1"
  version = "0.3.0"
}

provider "aws" {
  alias   = "another"
  region  = "eu-west-1"
  version = "0.3.0"
}

resource "aws_instance" "example" {

}

provider "google" {
  credentials = file("account.json")
  project     = "my-project-id"
  region      = "us-central1"
}

provider "aws" {
  alias  = "yet another"
  region = "eu-west-1"
}

output "hello" {
  value = "Hello, World"
}
`

const terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal = `
# Make sure comments are maintained
# And don't interfere with parsing
provider "aws" {
  # Make sure comments are maintained
  # And don't interfere with parsing
  region = var.aws_region

  # Make sure comments are maintained
  # And don't interfere with parsing
  version = "0.2.0"
}

# Make sure comments are maintained
# And don't interfere with parsing
provider "aws" {
  # Make sure comments are maintained
  # And don't interfere with parsing
  region = var.aws_region

  # Make sure comments are maintained
  # And don't interfere with parsing
  version = "0.2.0"

  # Make sure comments are maintained
  # And don't interfere with parsing
  alias = "secondary"
}
`

const terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsRegionOverriddenExpected = `
# Make sure comments are maintained
# And don't interfere with parsing
provider "aws" {
  # Make sure comments are maintained
  # And don't interfere with parsing
  region = "eu-west-1"

  # Make sure comments are maintained
  # And don't interfere with parsing
  version = "0.2.0"
}

# Make sure comments are maintained
# And don't interfere with parsing
provider "aws" {
  # Make sure comments are maintained
  # And don't interfere with parsing
  region = "eu-west-1"

  # Make sure comments are maintained
  # And don't interfere with parsing
  version = "0.2.0"

  # Make sure comments are maintained
  # And don't interfere with parsing
  alias = "secondary"
}
`

const terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsRegionVersionOverriddenExpected = `
# Make sure comments are maintained
# And don't interfere with parsing
provider "aws" {
  # Make sure comments are maintained
  # And don't interfere with parsing
  region = "eu-west-1"

  # Make sure comments are maintained
  # And don't interfere with parsing
  version = "0.3.0"
}

# Make sure comments are maintained
# And don't interfere with parsing
provider "aws" {
  # Make sure comments are maintained
  # And don't interfere with parsing
  region = "eu-west-1"

  # Make sure comments are maintained
  # And don't interfere with parsing
  version = "0.3.0"

  # Make sure comments are maintained
  # And don't interfere with parsing
  alias = "secondary"
}
`

const terraformCodeExampleAwsOneProviderNestedBlocks = `
provider "aws" {
  region = var.aws_region
  assume_role {
    role_arn = var.role_arn
  }
}
`

const terraformCodeExampleAwsOneProviderNestedBlocksRegionRoleArnExpected = `
provider "aws" {
  region = "eu-west-1"
  assume_role {
    role_arn = "nested-override"
  }
}
`

func TestPatchAwsProviderInTerraformCodeHappyPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		attributesToOverride   map[string]string
		testName               string
		originalTerraformCode  string
		expectedTerraformCode  []string
		expectedCodeWasUpdated bool
	}{
		{testName: "empty", originalTerraformCode: "", attributesToOverride: nil, expectedCodeWasUpdated: false, expectedTerraformCode: []string{""}},
		{testName: "empty with attributes", originalTerraformCode: "", attributesToOverride: map[string]string{"region": `"eu-west-1"`}, expectedCodeWasUpdated: false, expectedTerraformCode: []string{""}},
		{testName: "no provider", originalTerraformCode: terraformCodeExampleOutputOnly, attributesToOverride: map[string]string{"region": `"eu-west-1"`}, expectedCodeWasUpdated: false, expectedTerraformCode: []string{terraformCodeExampleOutputOnly}},
		{testName: "no aws provider", originalTerraformCode: terraformCodeExampleGcpProvider, attributesToOverride: map[string]string{"region": `"eu-west-1"`}, expectedCodeWasUpdated: false, expectedTerraformCode: []string{terraformCodeExampleGcpProvider}},
		{testName: "one empty aws provider, but no overrides", originalTerraformCode: terraformCodeExampleAwsProviderEmptyOriginal, attributesToOverride: nil, expectedCodeWasUpdated: false, expectedTerraformCode: []string{terraformCodeExampleAwsProviderEmptyOriginal}},
		{testName: "one empty aws provider, with region override", originalTerraformCode: terraformCodeExampleAwsProviderEmptyOriginal, attributesToOverride: map[string]string{"region": `"eu-west-1"`}, expectedCodeWasUpdated: false, expectedTerraformCode: []string{terraformCodeExampleAwsProviderEmptyOriginal}},
		{testName: "one empty aws provider, with region, version override", originalTerraformCode: terraformCodeExampleAwsProviderEmptyOriginal, attributesToOverride: map[string]string{"region": `"eu-west-1"`, "version": `"0.3.0"`}, expectedCodeWasUpdated: false, expectedTerraformCode: []string{terraformCodeExampleAwsProviderEmptyOriginal}},
		{testName: "one non-empty aws provider, but no overrides", originalTerraformCode: terraformCodeExampleAwsProviderNonEmptyOriginal, attributesToOverride: nil, expectedCodeWasUpdated: false, expectedTerraformCode: []string{terraformCodeExampleAwsProviderNonEmptyOriginal}},
		{testName: "one non-empty aws provider, with region override", originalTerraformCode: terraformCodeExampleAwsProviderNonEmptyOriginal, attributesToOverride: map[string]string{"region": `"eu-west-1"`}, expectedCodeWasUpdated: true, expectedTerraformCode: []string{terraformCodeExampleAwsProviderRegionOverridenVersionNotOverriddenExpected}},
		{testName: "one non-empty aws provider, with region, version override", originalTerraformCode: terraformCodeExampleAwsProviderNonEmptyOriginal, attributesToOverride: map[string]string{"region": `"eu-west-1"`, "version": `"0.3.0"`}, expectedCodeWasUpdated: true, expectedTerraformCode: []string{terraformCodeExampleAwsProviderRegionVersionOverridenExpected, terraformCodeExampleAwsProviderRegionVersionOverridenReverseOrderExpected}},
		{testName: "multiple providers, but no overrides", originalTerraformCode: terraformCodeExampleAwsMultipleProvidersOriginal, attributesToOverride: nil, expectedCodeWasUpdated: false, expectedTerraformCode: []string{terraformCodeExampleAwsMultipleProvidersOriginal}},
		{testName: "multiple providers, with region override", originalTerraformCode: terraformCodeExampleAwsMultipleProvidersOriginal, attributesToOverride: map[string]string{"region": `"eu-west-1"`}, expectedCodeWasUpdated: true, expectedTerraformCode: []string{terraformCodeExampleAwsMultipleProvidersRegionOverridenExpected}},
		{testName: "multiple providers, with region, version override", originalTerraformCode: terraformCodeExampleAwsMultipleProvidersOriginal, attributesToOverride: map[string]string{"region": `"eu-west-1"`, "version": `"0.3.0"`}, expectedCodeWasUpdated: true, expectedTerraformCode: []string{terraformCodeExampleAwsMultipleProvidersRegionVersionOverridenExpected}},
		{testName: "multiple providers with comments, but no overrides", originalTerraformCode: terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal, attributesToOverride: nil, expectedCodeWasUpdated: false, expectedTerraformCode: []string{terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal}},
		{testName: "multiple providers with comments, with region override", originalTerraformCode: terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal, attributesToOverride: map[string]string{"region": `"eu-west-1"`}, expectedCodeWasUpdated: true, expectedTerraformCode: []string{terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsRegionOverriddenExpected}},
		{testName: "multiple providers with comments, with region, version override", originalTerraformCode: terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal, attributesToOverride: map[string]string{"region": `"eu-west-1"`, "version": `"0.3.0"`}, expectedCodeWasUpdated: true, expectedTerraformCode: []string{terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsRegionVersionOverriddenExpected}},
		{testName: "one provider with nested blocks, with region and role_arn override", originalTerraformCode: terraformCodeExampleAwsOneProviderNestedBlocks, attributesToOverride: map[string]string{"region": `"eu-west-1"`, "assume_role.role_arn": `"nested-override"`}, expectedCodeWasUpdated: true, expectedTerraformCode: []string{terraformCodeExampleAwsOneProviderNestedBlocksRegionRoleArnExpected}},
		{testName: "one provider with nested blocks, with region and role_arn override, plus non-matching overrides", originalTerraformCode: terraformCodeExampleAwsOneProviderNestedBlocks, attributesToOverride: map[string]string{"region": `"eu-west-1"`, "assume_role.role_arn": `"nested-override"`, "should-be": `"ignored"`, "assume_role.should-be": `"ignored"`}, expectedCodeWasUpdated: true, expectedTerraformCode: []string{terraformCodeExampleAwsOneProviderNestedBlocksRegionRoleArnExpected}},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			t.Parallel()

			actualTerraformCode, actualCodeWasUpdated, err := awsproviderpatch.PatchAwsProviderInTerraformCode(tc.originalTerraformCode, "test.tf", tc.attributesToOverride)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedCodeWasUpdated, actualCodeWasUpdated)

			// We check an array  of possible expected code here due to possible ordering differences. That is, the
			// attributes within a provider block are stored in a map, and iteration order on maps is randomized, so
			// sometimes the provider block might come back with region first, followed by version, but other times,
			// the order is reversed. For those cases, we pass in multiple possible expected results and check that
			// one of them matches.
			assert.Contains(t, tc.expectedTerraformCode, actualTerraformCode)
		})
	}
}
