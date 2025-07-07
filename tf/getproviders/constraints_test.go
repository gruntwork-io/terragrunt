package getproviders_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf/getproviders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProviderConstraints(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	testDir := t.TempDir()

	// Create a test terraform file with required_providers block
	terraformContent := `
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
  }
}
`

	err := os.WriteFile(filepath.Join(testDir, "main.tf"), []byte(terraformContent), 0644)
	require.NoError(t, err)

	// Test parsing with Terraform implementation
	terraformOpts := &options.TerragruntOptions{
		TerraformImplementation: options.TerraformImpl,
	}
	constraints, err := getproviders.ParseProviderConstraints(terraformOpts, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints
	assert.Equal(t, "~> 5.0", constraints["registry.terraform.io/hashicorp/aws"])
	assert.Equal(t, "~> 4.0", constraints["registry.terraform.io/cloudflare/cloudflare"])

	// Test parsing with OpenTofu implementation
	openTofuOpts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
	}
	constraints, err = getproviders.ParseProviderConstraints(openTofuOpts, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints use OpenTofu registry
	assert.Equal(t, "~> 5.0", constraints["registry.opentofu.org/hashicorp/aws"])
	assert.Equal(t, "~> 4.0", constraints["registry.opentofu.org/cloudflare/cloudflare"])
}

func TestParseProviderConstraintsWithImplicitProvider(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	testDir := t.TempDir()

	// Create a test terraform file with implicit provider (no source specified)
	terraformContent := `
terraform {
  required_providers {
    aws = {
      version = "~> 5.0"
    }
  }
}
`

	err := os.WriteFile(filepath.Join(testDir, "main.tf"), []byte(terraformContent), 0644)
	require.NoError(t, err)

	// Test parsing with Terraform implementation
	terraformOpts := &options.TerragruntOptions{
		TerraformImplementation: options.TerraformImpl,
	}
	constraints, err := getproviders.ParseProviderConstraints(terraformOpts, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints default to terraform registry
	assert.Equal(t, "~> 5.0", constraints["registry.terraform.io/hashicorp/aws"])

	// Test parsing with OpenTofu implementation
	openTofuOpts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
	}
	constraints, err = getproviders.ParseProviderConstraints(openTofuOpts, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints default to OpenTofu registry
	assert.Equal(t, "~> 5.0", constraints["registry.opentofu.org/hashicorp/aws"])
}

func TestParseProviderConstraintsWithEnvironmentOverride(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	testDir := t.TempDir()

	// Create a test terraform file with implicit provider (no source specified)
	terraformContent := `
terraform {
  required_providers {
    aws = {
      version = "~> 5.0"
    }
    custom = {
      source  = "example/custom"
      version = "~> 1.0"
    }
  }
}
`

	err := os.WriteFile(filepath.Join(testDir, "main.tf"), []byte(terraformContent), 0644)
	require.NoError(t, err)

	// Set the environment variable to override the default registry
	customRegistry := "custom.registry.example.com"
	t.Setenv("TG_TF_DEFAULT_REGISTRY_HOST", customRegistry)

	// Test parsing with Terraform implementation - should use custom registry
	terraformOpts := &options.TerragruntOptions{
		TerraformImplementation: options.TerraformImpl,
	}
	constraints, err := getproviders.ParseProviderConstraints(terraformOpts, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints use custom registry for implicit providers
	assert.Equal(t, "~> 5.0", constraints[customRegistry+"/hashicorp/aws"])
	// Explicit source should use custom registry too
	assert.Equal(t, "~> 1.0", constraints[customRegistry+"/example/custom"])

	// Test parsing with OpenTofu implementation - should also use custom registry (environment override takes precedence)
	openTofuOpts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
	}
	constraints, err = getproviders.ParseProviderConstraints(openTofuOpts, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints use custom registry even with OpenTofu
	assert.Equal(t, "~> 5.0", constraints[customRegistry+"/hashicorp/aws"])
	assert.Equal(t, "~> 1.0", constraints[customRegistry+"/example/custom"])
}

func TestParseProviderConstraintsWithTofuFiles(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	testDir := t.TempDir()

	// Create a .tf file with one provider
	tfContent := `
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}
`
	err := os.WriteFile(filepath.Join(testDir, "main.tf"), []byte(tfContent), 0644)
	require.NoError(t, err)

	// Create a .tofu file with another provider
	tofuContent := `
terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }
}
`
	err = os.WriteFile(filepath.Join(testDir, "providers.tofu"), []byte(tofuContent), 0644)
	require.NoError(t, err)

	// Test parsing with OpenTofu implementation
	openTofuOpts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
	}
	constraints, err := getproviders.ParseProviderConstraints(openTofuOpts, testDir)
	require.NoError(t, err)

	// Verify constraints from both .tf and .tofu files are parsed
	assert.Equal(t, "~> 5.0", constraints["registry.opentofu.org/hashicorp/aws"])
	assert.Equal(t, "~> 3.0", constraints["registry.opentofu.org/hashicorp/azurerm"])

	// Test parsing with Terraform implementation
	terraformOpts := &options.TerragruntOptions{
		TerraformImplementation: options.TerraformImpl,
	}
	constraints, err = getproviders.ParseProviderConstraints(terraformOpts, testDir)
	require.NoError(t, err)

	// Verify constraints from both .tf and .tofu files are parsed with Terraform registry
	assert.Equal(t, "~> 5.0", constraints["registry.terraform.io/hashicorp/aws"])
	assert.Equal(t, "~> 3.0", constraints["registry.terraform.io/hashicorp/azurerm"])
}
