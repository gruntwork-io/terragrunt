//go:build tofu

package run_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTofuBackendDetectionWithRegex(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		description   string
		files         map[string]string
		backendType   string
		expectBackend bool
	}{
		{
			description: "Backend in .tofu file",
			files: map[string]string{
				"main.tofu": `
terraform {
  backend "s3" {
    bucket = "my-terraform-state"
    key    = "opentofu/terraform.tfstate"
    region = "us-west-2"
  }
}

resource "aws_instance" "example" {
  ami           = "ami-0c55b159cbfafe1d0"
  instance_type = "t2.micro"
}`,
			},
			backendType:   "s3",
			expectBackend: true,
		},
		{
			description: "Backend in mixed .tf/.tofu files - backend in .tf",
			files: map[string]string{
				"backend.tf": `
terraform {
  required_version = ">= 1.0"

  backend "s3" {
    bucket = "terraform-state-bucket"
    key    = "mixed/terraform.tfstate"
    region = "us-east-1"
  }
}`,
				"resources.tofu": `
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}`,
			},
			backendType:   "s3",
			expectBackend: true,
		},
		{
			description: "No backend in .tofu files",
			files: map[string]string{
				"main.tofu": `
resource "aws_instance" "example" {
  ami           = "ami-0c55b159cbfafe1d0"
  instance_type = "t2.micro"
}`,
			},
			backendType:   "s3",
			expectBackend: false,
		},
		{
			description: "Wrong backend type in .tofu files",
			files: map[string]string{
				"main.tofu": `
terraform {
  backend "s3" {
    bucket = "my-terraform-state"
    key    = "opentofu/terraform.tfstate"
    region = "us-west-2"
  }
}`,
			},
			backendType:   "gcs",
			expectBackend: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			for filename, content := range tc.files {
				filePath := filepath.Join(tmpDir, filename)
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
			}

			terraformBackendRegexp := regexp.MustCompile(fmt.Sprintf(`backend[[:blank:]]+"%s"`, tc.backendType))

			hasBackend, err := util.RegexFoundInTFFiles(tmpDir, terraformBackendRegexp)
			require.NoError(t, err)

			assert.Equal(t, tc.expectBackend, hasBackend, "For test case: %s", tc.description)
		})
	}
}

func TestTofuModuleDetectionWithRegex(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files         map[string]string
		description   string
		expectModules bool
	}{
		{
			description: "Modules in .tofu file",
			files: map[string]string{
				"main.tofu": `
module "vpc" {
  source = "./modules/vpc"

  cidr_block = "10.0.0.0/16"
}

module "security_group" {
  source = "git::https://github.com/example/terraform-modules.git//security-group"

  vpc_id = module.vpc.vpc_id
}

output "vpc_id" {
  value = module.vpc.vpc_id
}`,
			},
			expectModules: true,
		},
		{
			description: "Modules in mixed .tf/.tofu files",
			files: map[string]string{
				"main.tf": `
module "network" {
  source = "./modules/network"

  vpc_cidr = "10.0.0.0/16"
}`,
				"compute.tofu": `
module "web_servers" {
  source = "git::https://github.com/example/terraform-modules.git//web-server"

  vpc_id = module.network.vpc_id
}`,
			},
			expectModules: true,
		},
		{
			description: "No modules in .tofu files",
			files: map[string]string{
				"main.tofu": `
resource "aws_instance" "example" {
  ami           = "ami-0c55b159cbfafe1d0"
  instance_type = "t2.micro"
}`,
			},
			expectModules: false,
		},
		{
			description: "Backend only (no modules) in .tofu files",
			files: map[string]string{
				"main.tofu": `
terraform {
  backend "s3" {
    bucket = "my-terraform-state"
    key    = "opentofu/terraform.tfstate"
    region = "us-west-2"
  }
}

resource "aws_instance" "example" {
  ami           = "ami-0c55b159cbfafe1d0"
  instance_type = "t2.micro"
}`,
			},
			expectModules: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			for filename, content := range tc.files {
				filePath := filepath.Join(tmpDir, filename)
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
			}

			moduleRegex := regexp.MustCompile(`module[[:blank:]]+".+"`)

			hasModules, err := util.RegexFoundInTFFiles(tmpDir, moduleRegex)
			require.NoError(t, err)

			assert.Equal(t, tc.expectModules, hasModules, "For test case: %s", tc.description)
		})
	}
}

func TestTofuCodeCheck(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files       map[string]string
		description string
		expectValid bool
	}{
		{
			description: "Directory with .tofu backend file",
			files: map[string]string{
				"main.tofu": `
terraform {
  backend "s3" {
    bucket = "my-terraform-state"
    key    = "opentofu/terraform.tfstate"
    region = "us-west-2"
  }
}

resource "aws_instance" "example" {
  ami           = "ami-0c55b159cbfafe1d0"
  instance_type = "t2.micro"
}`,
			},
			expectValid: true,
		},
		{
			description: "Directory with .tofu modules file",
			files: map[string]string{
				"main.tofu": `
module "vpc" {
  source = "./modules/vpc"
  cidr_block = "10.0.0.0/16"
}`,
			},
			expectValid: true,
		},
		{
			description: "Directory with mixed .tf/.tofu files",
			files: map[string]string{
				"backend.tf": `
terraform {
  backend "s3" {
    bucket = "terraform-state-bucket"
    key    = "mixed/terraform.tfstate"
    region = "us-east-1"
  }
}`,
				"resources.tofu": `
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}`,
			},
			expectValid: true,
		},
		{
			description: "Directory with existing .tofu file",
			files: map[string]string{
				"main.tofu": `# Simple tofu file`,
			},
			expectValid: true,
		},
		{
			description: "Directory with existing .tf file",
			files: map[string]string{
				"main.tf": `# Simple tf file`,
			},
			expectValid: true,
		},
		{
			description: "Directory with both .tf and .tofu files",
			files: map[string]string{
				"main.tf":   `# Terraform file`,
				"main.tofu": `# OpenTofu file`,
			},
			expectValid: true,
		},
		{
			description: "Directory with no Terraform/OpenTofu files",
			files: map[string]string{
				"main.yaml": `# Not a terraform file`,
			},
			expectValid: false,
		},
		{
			description: "Empty directory",
			files:       map[string]string{},
			expectValid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			for filename, content := range tc.files {
				filePath := filepath.Join(tmpDir, filename)
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
			}

			opts, err := options.NewTerragruntOptionsForTest("mock-path-for-test.hcl")
			require.NoError(t, err)

			opts.WorkingDir = tmpDir

			err = run.CheckFolderContainsTerraformCode(opts)

			if tc.expectValid {
				assert.NoError(t, err, "Expected no error for valid directory: %s", tc.description)
			} else {
				assert.Error(t, err, "Expected error for invalid directory: %s", tc.description)
			}
		})
	}
}

func TestTofuCacheValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files          map[string]string
		description    string
		expectHasFiles bool
		expectError    bool
	}{
		{
			description: "Directory with .tofu files should be detected",
			files: map[string]string{
				"main.tofu": `# Simple tofu file`,
			},
			expectHasFiles: true,
			expectError:    false,
		},
		{
			description: "Directory with .tofu backend files should be detected",
			files: map[string]string{
				"main.tofu": `
terraform {
  backend "s3" {
    bucket = "my-terraform-state"
    key    = "opentofu/terraform.tfstate"
    region = "us-west-2"
  }
}`,
			},
			expectHasFiles: true,
			expectError:    false,
		},
		{
			description: "Directory with mixed files should be detected",
			files: map[string]string{
				"backend.tf": `
terraform {
  backend "s3" {
    bucket = "terraform-state-bucket"
    key    = "mixed/terraform.tfstate"
    region = "us-east-1"
  }
}`,
				"resources.tofu": `
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}`,
			},
			expectHasFiles: true,
			expectError:    false,
		},
		{
			description: "Directory with no TF files should not be detected",
			files: map[string]string{
				"main.yaml": `# Not a terraform file`,
				"script.sh": `#!/bin/bash\necho "hello"`,
			},
			expectHasFiles: false,
			expectError:    false,
		},
		{
			description:    "Empty directory should not be detected",
			files:          map[string]string{},
			expectHasFiles: false,
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			for filename, content := range tc.files {
				filePath := filepath.Join(tmpDir, filename)
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
			}

			hasFiles, err := util.DirContainsTFFiles(tmpDir)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectHasFiles, hasFiles, "For test case: %s", tc.description)
			}
		})
	}
}
