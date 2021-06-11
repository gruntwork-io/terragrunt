package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

const terraformCodeExampleAwsProviderRegionOverridenExpected = `
provider "aws" {
  region = "eu-west-1"
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
		testName               string
		originalTerraformCode  string
		attributesToOverride   map[string]string
		expectedCodeWasUpdated bool
		expectedTerraformCode  []string
	}{
		{"empty", "", nil, false, []string{""}},
		{"empty with attributes", "", map[string]string{"region": `"eu-west-1"`}, false, []string{""}},
		{"no provider", terraformCodeExampleOutputOnly, map[string]string{"region": `"eu-west-1"`}, false, []string{terraformCodeExampleOutputOnly}},
		{"no aws provider", terraformCodeExampleGcpProvider, map[string]string{"region": `"eu-west-1"`}, false, []string{terraformCodeExampleGcpProvider}},
		{"one empty aws provider, but no overrides", terraformCodeExampleAwsProviderEmptyOriginal, nil, false, []string{terraformCodeExampleAwsProviderEmptyOriginal}},
		{"one empty aws provider, with region override", terraformCodeExampleAwsProviderEmptyOriginal, map[string]string{"region": `"eu-west-1"`}, false, []string{terraformCodeExampleAwsProviderEmptyOriginal}},
		{"one empty aws provider, with region, version override", terraformCodeExampleAwsProviderEmptyOriginal, map[string]string{"region": `"eu-west-1"`, "version": `"0.3.0"`}, false, []string{terraformCodeExampleAwsProviderEmptyOriginal}},
		{"one non-empty aws provider, but no overrides", terraformCodeExampleAwsProviderNonEmptyOriginal, nil, false, []string{terraformCodeExampleAwsProviderNonEmptyOriginal}},
		{"one non-empty aws provider, with region override", terraformCodeExampleAwsProviderNonEmptyOriginal, map[string]string{"region": `"eu-west-1"`}, true, []string{terraformCodeExampleAwsProviderRegionOverridenVersionNotOverriddenExpected}},
		{"one non-empty aws provider, with region, version override", terraformCodeExampleAwsProviderNonEmptyOriginal, map[string]string{"region": `"eu-west-1"`, "version": `"0.3.0"`}, true, []string{terraformCodeExampleAwsProviderRegionVersionOverridenExpected, terraformCodeExampleAwsProviderRegionVersionOverridenReverseOrderExpected}},
		{"multiple providers, but no overrides", terraformCodeExampleAwsMultipleProvidersOriginal, nil, false, []string{terraformCodeExampleAwsMultipleProvidersOriginal}},
		{"multiple providers, with region override", terraformCodeExampleAwsMultipleProvidersOriginal, map[string]string{"region": `"eu-west-1"`}, true, []string{terraformCodeExampleAwsMultipleProvidersRegionOverridenExpected}},
		{"multiple providers, with region, version override", terraformCodeExampleAwsMultipleProvidersOriginal, map[string]string{"region": `"eu-west-1"`, "version": `"0.3.0"`}, true, []string{terraformCodeExampleAwsMultipleProvidersRegionVersionOverridenExpected}},
		{"multiple providers with comments, but no overrides", terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal, nil, false, []string{terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal}},
		{"multiple providers with comments, with region override", terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal, map[string]string{"region": `"eu-west-1"`}, true, []string{terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsRegionOverriddenExpected}},
		{"multiple providers with comments, with region, version override", terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal, map[string]string{"region": `"eu-west-1"`, "version": `"0.3.0"`}, true, []string{terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsRegionVersionOverriddenExpected}},
		{"one provider with nested blocks, with region and role_arn override", terraformCodeExampleAwsOneProviderNestedBlocks, map[string]string{"region": `"eu-west-1"`, "assume_role.role_arn": `"nested-override"`}, true, []string{terraformCodeExampleAwsOneProviderNestedBlocksRegionRoleArnExpected}},
		{"one provider with nested blocks, with region and role_arn override, plus non-matching overrides", terraformCodeExampleAwsOneProviderNestedBlocks, map[string]string{"region": `"eu-west-1"`, "assume_role.role_arn": `"nested-override"`, "should-be": `"ignored"`, "assume_role.should-be": `"ignored"`}, true, []string{terraformCodeExampleAwsOneProviderNestedBlocksRegionRoleArnExpected}},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase
		t.Run(testCase.testName, func(t *testing.T) {
			t.Parallel()
			actualTerraformCode, actualCodeWasUpdated, err := patchAwsProviderInTerraformCode(testCase.originalTerraformCode, "test.tf", testCase.attributesToOverride)
			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedCodeWasUpdated, actualCodeWasUpdated)

			// We check an array  of possible expected code here due to possible ordering differences. That is, the
			// attributes within a provider block are stored in a map, and iteration order on maps is randomized, so
			// sometimes the provider block might come back with region first, followed by version, but other times,
			// the order is reversed. For those cases, we pass in multiple possible expected results and check that
			// one of them matches.
			assert.Contains(t, testCase.expectedTerraformCode, actualTerraformCode)
		})
	}
}
