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

	assert.Equal(t, "~> 5.0.0", constraints["registry.terraform.io/hashicorp/aws"])
	assert.Equal(t, "~> 4.0.0", constraints["registry.terraform.io/cloudflare/cloudflare"])

	// Test parsing with OpenTofu implementation
	openTofuOpts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
	}
	constraints, err = getproviders.ParseProviderConstraints(openTofuOpts, testDir)
	require.NoError(t, err)

	assert.Equal(t, "~> 5.0.0", constraints["registry.opentofu.org/hashicorp/aws"])
	assert.Equal(t, "~> 4.0.0", constraints["registry.opentofu.org/cloudflare/cloudflare"])
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

	// Verify the parsed constraints default to terraform registry and are normalized
	assert.Equal(t, "~> 5.0.0", constraints["registry.terraform.io/hashicorp/aws"])

	// Test parsing with OpenTofu implementation
	openTofuOpts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
	}
	constraints, err = getproviders.ParseProviderConstraints(openTofuOpts, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints default to OpenTofu registry and are normalized
	assert.Equal(t, "~> 5.0.0", constraints["registry.opentofu.org/hashicorp/aws"])
}

func TestParseProviderConstraintsWithEnvironmentOverride(t *testing.T) {
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

	// Verify the parsed constraints use custom registry for implicit providers and are normalized
	assert.Equal(t, "~> 5.0.0", constraints[customRegistry+"/hashicorp/aws"])
	// Explicit source should use custom registry too and be normalized
	assert.Equal(t, "~> 1.0.0", constraints[customRegistry+"/example/custom"])

	// Test parsing with OpenTofu implementation - should also use custom registry (environment override takes precedence)
	openTofuOpts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
	}
	constraints, err = getproviders.ParseProviderConstraints(openTofuOpts, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints use custom registry even with OpenTofu and are normalized
	assert.Equal(t, "~> 5.0.0", constraints[customRegistry+"/hashicorp/aws"])
	assert.Equal(t, "~> 1.0.0", constraints[customRegistry+"/example/custom"])
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

	// Verify constraints from both .tf and .tofu files are parsed and normalized
	assert.Equal(t, "~> 5.0.0", constraints["registry.opentofu.org/hashicorp/aws"])
	assert.Equal(t, "~> 3.0.0", constraints["registry.opentofu.org/hashicorp/azurerm"])

	// Test parsing with Terraform implementation
	terraformOpts := &options.TerragruntOptions{
		TerraformImplementation: options.TerraformImpl,
	}
	constraints, err = getproviders.ParseProviderConstraints(terraformOpts, testDir)
	require.NoError(t, err)

	// Verify constraints from both .tf and .tofu files are parsed with Terraform registry and normalized
	assert.Equal(t, "~> 5.0.0", constraints["registry.terraform.io/hashicorp/aws"])
	assert.Equal(t, "~> 3.0.0", constraints["registry.terraform.io/hashicorp/azurerm"])
}

func TestParseProviderConstraintsWithEqualsPrefix(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	testDir := t.TempDir()

	// Create a test terraform file with "=" prefix in version constraints
	terraformContent := `
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "= 5.100.0"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "= 4.40.0"
    }
    time = {
      source  = "hashicorp/time"
      version = ">= 0.10.0"
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

	// Verify the parsed constraints are normalized (no "=" prefix)
	assert.Equal(t, "5.100.0", constraints["registry.terraform.io/hashicorp/aws"])
	assert.Equal(t, "4.40.0", constraints["registry.terraform.io/cloudflare/cloudflare"])
	assert.Equal(t, ">= 0.10.0", constraints["registry.terraform.io/hashicorp/time"])

	// Test parsing with OpenTofu implementation
	openTofuOpts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
	}
	constraints, err = getproviders.ParseProviderConstraints(openTofuOpts, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints are normalized with OpenTofu registry
	assert.Equal(t, "5.100.0", constraints["registry.opentofu.org/hashicorp/aws"])
	assert.Equal(t, "4.40.0", constraints["registry.opentofu.org/cloudflare/cloudflare"])
	assert.Equal(t, ">= 0.10.0", constraints["registry.opentofu.org/hashicorp/time"])
}

func TestNormalizeVersionConstraint(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normalize basic version constraint",
			input:    ">= 2.2",
			expected: ">= 2.2.0",
		},
		{
			name:     "normalize pessimistic constraint",
			input:    "~> 4.0",
			expected: "~> 4.0.0",
		},
		{
			name:     "already normalized constraint unchanged",
			input:    ">= 2.2.0",
			expected: ">= 2.2.0",
		},
		{
			name:     "remove equals prefix",
			input:    "= 1.0",
			expected: "1.0.0",
		},
		{
			name:     "complex constraint with patch version",
			input:    "~> 3.14.15",
			expected: "~> 3.14.15",
		},
		{
			name:     "exact version constraint",
			input:    "1.2",
			expected: "1.2.0",
		},
		{
			name:     "invalid constraint returned as-is",
			input:    "invalid-constraint",
			expected: "invalid-constraint",
		},
		{
			name:     "whitespace handling",
			input:    "  >= 1.0  ",
			expected: ">= 1.0.0",
		},
		{
			name:     "equals prefix with whitespace",
			input:    "= 2.5",
			expected: "2.5.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// We need to call the unexported function through the public API
			// So we'll test it through the constraint parsing
			testDir := t.TempDir()
			terraformContent := `terraform {
  required_providers {
    test = {
      source  = "example/test"
      version = "` + tc.input + `"
    }
  }
}`

			err := os.WriteFile(filepath.Join(testDir, "main.tf"), []byte(terraformContent), 0644)
			require.NoError(t, err)

			opts := &options.TerragruntOptions{
				TerraformImplementation: options.TerraformImpl,
			}
			constraints, err := getproviders.ParseProviderConstraints(opts, testDir)
			require.NoError(t, err)

			result := constraints["registry.terraform.io/example/test"]
			assert.Equal(t, tc.expected, result, "Input: %s", tc.input)
		})
	}
}
