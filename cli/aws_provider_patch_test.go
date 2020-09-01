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

func TestPatchAwsProviderInTerraformCodeHappyPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		testName              string
		originalTerraformCode string
		attributesToOverride  map[string]string
		expectedTerraformCode string
	}{
		{"empty", "", nil, ""},
		{"empty with attributes", "", map[string]string{"region": "eu-west-1"}, ""},
		{"no provider", terraformCodeExampleOutputOnly, map[string]string{"region": "eu-west-1"}, terraformCodeExampleOutputOnly},
		{"no aws provider", terraformCodeExampleGcpProvider, map[string]string{"region": "eu-west-1"}, terraformCodeExampleGcpProvider},
		{"one empty aws provider, but no overrides", terraformCodeExampleAwsProviderEmptyOriginal, nil, terraformCodeExampleAwsProviderEmptyOriginal},
		{"one empty aws provider, with region override", terraformCodeExampleAwsProviderEmptyOriginal, map[string]string{"region": "eu-west-1"}, terraformCodeExampleAwsProviderRegionOverridenExpected},
		{"one empty aws provider, with region, version, profile override", terraformCodeExampleAwsProviderEmptyOriginal, map[string]string{"region": "eu-west-1", "version": "0.3.0", "profile": "foo"}, terraformCodeExampleAwsProviderRegionVersionProfileOverridenExpected},
		{"one non-empty aws provider, but no overrides", terraformCodeExampleAwsProviderNonEmptyOriginal, nil, terraformCodeExampleAwsProviderNonEmptyOriginal},
		{"one non-empty aws provider, with region override", terraformCodeExampleAwsProviderNonEmptyOriginal, map[string]string{"region": "eu-west-1"}, terraformCodeExampleAwsProviderRegionOverridenVersionNotOverriddenExpected},
		{"one non-empty aws provider, with region, version, profile override", terraformCodeExampleAwsProviderNonEmptyOriginal, map[string]string{"region": "eu-west-1", "version": "0.3.0", "profile": "foo"}, terraformCodeExampleAwsProviderRegionVersionProfileOverridenExpected},
		{"multiple providers, but no overrides", terraformCodeExampleAwsMultipleProvidersOriginal, nil, terraformCodeExampleAwsMultipleProvidersOriginal},
		{"multiple providers, with region override", terraformCodeExampleAwsMultipleProvidersOriginal, map[string]string{"region": "eu-west-1"}, terraformCodeExampleAwsMultipleProvidersRegionOverridenExpected},
		{"multiple providers, with region, version, profile override", terraformCodeExampleAwsMultipleProvidersOriginal, map[string]string{"region": "eu-west-1", "version": "0.3.0", "profile": "foo"}, terraformCodeExampleAwsMultipleProvidersRegionProfileVersionOverridenExpected},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase
		t.Run(testCase.testName, func(t *testing.T) {
			t.Parallel()
			actualTerraformCode, err := patchAwsProviderInTerraformCode(testCase.originalTerraformCode, "test.tf", testCase.attributesToOverride)
			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedTerraformCode, actualTerraformCode)
		})
	}
}
