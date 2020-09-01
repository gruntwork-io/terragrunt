package cli

import (
	"github.com/stretchr/testify/assert"
	"testing"
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

const terraformCodeExampleAwsProviderRegionVersionProfileOverridenExpected = `
provider "aws" {
  region  = "eu-west-1"
  version = "0.3.0"
  profile = "foo"
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

const terraformCodeExampleAwsMultipleProvidersRegionProfileVersionOverridenExpected = `
provider "aws" {
  region  = "eu-west-1"
  version = "0.3.0"
  profile = "foo"
}

provider "aws" {
  alias   = "another"
  region  = "eu-west-1"
  version = "0.3.0"
  profile = "foo"
}

resource "aws_instance" "example" {

}

provider "google" {
  credentials = file("account.json")
  project     = "my-project-id"
  region      = "us-central1"
}

provider "aws" {
  alias   = "yet another"
  region  = "eu-west-1"
  version = "0.3.0"
  profile = "foo"
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

const terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsRegionVersionProfileOverriddenExpected = `
# Make sure comments are maintained
# And don't interfere with parsing
provider "aws" {
  # Make sure comments are maintained
  # And don't interfere with parsing
  region = "eu-west-1"

  # Make sure comments are maintained
  # And don't interfere with parsing
  version = "0.3.0"
  profile = "foo"
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
  alias   = "secondary"
  profile = "foo"
}
`

func TestPatchAwsProviderInTerraformCodeHappyPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		testName               string
		originalTerraformCode  string
		attributesToOverride   map[string]string
		expectedCodeWasUpdated bool
		expectedTerraformCode  string
	}{
		{"empty", "", nil, false, ""},
		{"empty with attributes", "", map[string]string{"region": "eu-west-1"}, false, ""},
		{"no provider", terraformCodeExampleOutputOnly, map[string]string{"region": "eu-west-1"}, false, terraformCodeExampleOutputOnly},
		{"no aws provider", terraformCodeExampleGcpProvider, map[string]string{"region": "eu-west-1"}, false, terraformCodeExampleGcpProvider},
		{"one empty aws provider, but no overrides", terraformCodeExampleAwsProviderEmptyOriginal, nil, false, terraformCodeExampleAwsProviderEmptyOriginal},
		{"one empty aws provider, with region override", terraformCodeExampleAwsProviderEmptyOriginal, map[string]string{"region": "eu-west-1"}, true, terraformCodeExampleAwsProviderRegionOverridenExpected},
		{"one empty aws provider, with region, version, profile override", terraformCodeExampleAwsProviderEmptyOriginal, map[string]string{"region": "eu-west-1", "version": "0.3.0", "profile": "foo"}, true, terraformCodeExampleAwsProviderRegionVersionProfileOverridenExpected},
		{"one non-empty aws provider, but no overrides", terraformCodeExampleAwsProviderNonEmptyOriginal, nil, false, terraformCodeExampleAwsProviderNonEmptyOriginal},
		{"one non-empty aws provider, with region override", terraformCodeExampleAwsProviderNonEmptyOriginal, map[string]string{"region": "eu-west-1"}, true, terraformCodeExampleAwsProviderRegionOverridenVersionNotOverriddenExpected},
		{"one non-empty aws provider, with region, version, profile override", terraformCodeExampleAwsProviderNonEmptyOriginal, map[string]string{"region": "eu-west-1", "version": "0.3.0", "profile": "foo"}, true, terraformCodeExampleAwsProviderRegionVersionProfileOverridenExpected},
		{"multiple providers, but no overrides", terraformCodeExampleAwsMultipleProvidersOriginal, nil, false, terraformCodeExampleAwsMultipleProvidersOriginal},
		{"multiple providers, with region override", terraformCodeExampleAwsMultipleProvidersOriginal, map[string]string{"region": "eu-west-1"}, true, terraformCodeExampleAwsMultipleProvidersRegionOverridenExpected},
		{"multiple providers, with region, version, profile override", terraformCodeExampleAwsMultipleProvidersOriginal, map[string]string{"region": "eu-west-1", "version": "0.3.0", "profile": "foo"}, true, terraformCodeExampleAwsMultipleProvidersRegionProfileVersionOverridenExpected},
		{"multiple providers with comments, but no overrides", terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal, nil, false, terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal},
		{"multiple providers with comments, with region override", terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal, map[string]string{"region": "eu-west-1"}, true, terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsRegionOverriddenExpected},
		{"multiple providers with comments, with region, version, profile override", terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsOriginal, map[string]string{"region": "eu-west-1", "version": "0.3.0", "profile": "foo"}, true, terraformCodeExampleAwsMultipleProvidersNonEmptyWithCommentsRegionVersionProfileOverriddenExpected},
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
			assert.Equal(t, testCase.expectedTerraformCode, actualTerraformCode)
		})
	}
}
