//go:build tofu

package awsproviderpatch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tofuCodeExampleAwsProviderOriginal = `
provider "aws" {
  region = var.aws_region
}

resource "aws_instance" "example" {
  ami           = "ami-0c55b159cbfafe1d0"
  instance_type = "t2.micro"
}
`

const tofuCodeExampleAwsProviderRegionOverridden = `
provider "aws" {
  region = "eu-west-1"
}

resource "aws_instance" "example" {
  ami           = "ami-0c55b159cbfafe1d0"
  instance_type = "t2.micro"
}
`

const tofuCodeExampleMultipleProvidersOriginal = `
provider "aws" {
  region = var.aws_region
}

provider "aws" {
  alias  = "east"
  region = "us-east-1"
}

provider "google" {
  project = "my-project"
  region  = "us-central1"
}

resource "aws_instance" "example" {
  ami           = "ami-0c55b159cbfafe1d0"
  instance_type = "t2.micro"
}
`

const tofuCodeExampleMultipleProvidersRegionOverridden = `
provider "aws" {
  region = "eu-west-1"
}

provider "aws" {
  alias  = "east"
  region = "eu-west-1"
}

provider "google" {
  project = "my-project"
  region  = "us-central1"
}

resource "aws_instance" "example" {
  ami           = "ami-0c55b159cbfafe1d0"
  instance_type = "t2.micro"
}
`

const tofuCodeExampleNestedBlocksOriginal = `
provider "aws" {
  region = var.aws_region

  assume_role {
    role_arn = "arn:aws:iam::123456789012:role/example"
  }

  default_tags {
    tags = {
      Environment = "test"
    }
  }
}
`

const tofuCodeExampleNestedBlocksRegionRoleArnOverridden = `
provider "aws" {
  region = "eu-west-1"

  assume_role {
    role_arn = "arn:aws:iam::123456789012:role/overridden"
  }

  default_tags {
    tags = {
      Environment = "test"
    }
  }
}
`

func TestPatchAwsProviderInTofuCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		testName               string
		originalTofuCode       string
		attributesToOverride   map[string]string
		expectedTofuCode       []string
		expectedCodeWasUpdated bool
	}{
		{
			testName:             "empty tofu file",
			attributesToOverride: map[string]string{"region": `"eu-west-1"`},
			expectedTofuCode:     []string{""},
		},
		{
			testName:             "tofu file with no aws provider",
			originalTofuCode:     `resource "null_resource" "example" {}`,
			attributesToOverride: map[string]string{"region": `"eu-west-1"`},
			expectedTofuCode:     []string{`resource "null_resource" "example" {}`},
		},
		{
			testName:               "tofu file with aws provider - region override",
			originalTofuCode:       tofuCodeExampleAwsProviderOriginal,
			attributesToOverride:   map[string]string{"region": `"eu-west-1"`},
			expectedCodeWasUpdated: true,
			expectedTofuCode:       []string{tofuCodeExampleAwsProviderRegionOverridden},
		},
		{
			testName:               "tofu file with multiple aws providers - region override",
			originalTofuCode:       tofuCodeExampleMultipleProvidersOriginal,
			attributesToOverride:   map[string]string{"region": `"eu-west-1"`},
			expectedCodeWasUpdated: true,
			expectedTofuCode:       []string{tofuCodeExampleMultipleProvidersRegionOverridden},
		},
		{
			testName:               "tofu file with nested blocks - region and role_arn override",
			originalTofuCode:       tofuCodeExampleNestedBlocksOriginal,
			attributesToOverride:   map[string]string{"region": `"eu-west-1"`, "assume_role.role_arn": `"arn:aws:iam::123456789012:role/overridden"`},
			expectedCodeWasUpdated: true,
			expectedTofuCode:       []string{tofuCodeExampleNestedBlocksRegionRoleArnOverridden},
		},
		{
			testName:         "tofu file with aws provider - no overrides",
			originalTofuCode: tofuCodeExampleAwsProviderOriginal,
			expectedTofuCode: []string{tofuCodeExampleAwsProviderOriginal},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			t.Parallel()

			actualTofuCode, actualCodeWasUpdated, err := awsproviderpatch.PatchAwsProviderInTerraformCode(
				tc.originalTofuCode,
				"test.tofu",
				tc.attributesToOverride,
			)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedCodeWasUpdated, actualCodeWasUpdated)
			assert.Contains(t, tc.expectedTofuCode, actualTofuCode)
		})
	}
}

func TestFindAllTerraformFilesIncludesTofuFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	terraformModulesDir := filepath.Join(tmpDir, ".terraform", "modules")
	require.NoError(t, os.MkdirAll(terraformModulesDir, 0755))

	modulesJSON := `{
		"Modules": [
			{
				"Key": "",
				"Source": "",
				"Dir": "."
			},
			{
				"Key": "vpc",
				"Source": "./modules/vpc",
				"Dir": "modules/vpc"
			},
			{
				"Key": "security",
				"Source": "./modules/security",
				"Dir": "modules/security"
			}
		]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(terraformModulesDir, "modules.json"), []byte(modulesJSON), 0644))

	modules := map[string][]string{
		"modules/vpc":      {"main.tf", "variables.tofu", "outputs.tf.json"},
		"modules/security": {"main.tofu", "variables.tf", "data.tofu.json"},
	}

	for moduleDir, files := range modules {
		modulePath := filepath.Join(tmpDir, moduleDir)
		require.NoError(t, os.MkdirAll(modulePath, 0755))

		for _, file := range files {
			filePath := filepath.Join(modulePath, file)
			content := "# Test content for " + file
			require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
		}
	}

	opts, err := options.NewTerragruntOptionsForTest("test.hcl")
	require.NoError(t, err)

	opts.WorkingDir = tmpDir

	allFiles, err := util.FindTFFiles(tmpDir)
	require.NoError(t, err)

	var files []string

	for _, file := range allFiles {
		if !strings.HasSuffix(file, ".json") {
			files = append(files, file)
		}
	}

	expectedFiles := []string{
		filepath.Join(tmpDir, "modules/vpc/main.tf"),
		filepath.Join(tmpDir, "modules/vpc/variables.tofu"),
		filepath.Join(tmpDir, "modules/security/main.tofu"),
		filepath.Join(tmpDir, "modules/security/variables.tf"),
	}

	assert.Len(t, files, len(expectedFiles))

	for _, expectedFile := range expectedFiles {
		assert.Contains(t, files, expectedFile, "Expected file %s not found in results", expectedFile)
	}

	for _, file := range files {
		assert.NotEqual(t, ".json", filepath.Ext(file), "JSON file %s should be excluded", file)
	}
}

func TestAwsProviderPatchWithMixedFileTypes(t *testing.T) {
	t.Parallel()

	tfContent := `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = "us-west-2"
}`

	modifiedTfContent, wasUpdated, err := awsproviderpatch.PatchAwsProviderInTerraformCode(
		tfContent,
		"main.tf",
		map[string]string{"region": `"eu-west-1"`},
	)
	require.NoError(t, err)
	assert.True(t, wasUpdated)
	assert.Contains(t, modifiedTfContent, `region = "eu-west-1"`)

	tofuContent := `provider "aws" {
  alias  = "secondary"
  region = var.secondary_region
}

resource "aws_s3_bucket" "primary" {
  bucket = "${var.environment}-primary-bucket"
}`

	modifiedTofuContent, wasUpdated, err := awsproviderpatch.PatchAwsProviderInTerraformCode(
		tofuContent,
		"resources.tofu",
		map[string]string{"region": `"eu-west-1"`},
	)
	require.NoError(t, err)
	assert.True(t, wasUpdated)
	assert.Contains(t, modifiedTofuContent, `region = "eu-west-1"`)
}

func TestTofuFileExtensionRecognition(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		filename         string
		description      string
		shouldBeIncluded bool
	}{
		{filename: "main.tf", shouldBeIncluded: true, description: "Standard Terraform file"},
		{filename: "main.tofu", shouldBeIncluded: true, description: "OpenTofu file"},
		{filename: "variables.tf.json", shouldBeIncluded: true, description: "Terraform JSON file (recognized but filtered out during processing)"},
		{filename: "variables.tofu.json", shouldBeIncluded: true, description: "OpenTofu JSON file (recognized but filtered out during processing)"},
		{filename: "outputs.tf", shouldBeIncluded: true, description: "Terraform outputs file"},
		{filename: "outputs.tofu", shouldBeIncluded: true, description: "OpenTofu outputs file"},
		{filename: "providers.tf", shouldBeIncluded: true, description: "Terraform providers file"},
		{filename: "providers.tofu", shouldBeIncluded: true, description: "OpenTofu providers file"},
		{filename: "terraform.tfvars", shouldBeIncluded: false, description: "Terraform variables file (not a configuration file)"},
		{filename: "terragrunt.hcl", shouldBeIncluded: false, description: "Terragrunt configuration file"},
		{filename: "README.md", shouldBeIncluded: false, description: "Documentation file"},
		{filename: "script.sh", shouldBeIncluded: false, description: "Shell script"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			actualResult := util.IsTFFile(tc.filename)

			if tc.shouldBeIncluded {
				assert.True(t, actualResult, "File %s should be recognized as a TF file", tc.filename)
			} else {
				assert.False(t, actualResult, "File %s should not be recognized as a TF file", tc.filename)
			}
		})
	}
}
