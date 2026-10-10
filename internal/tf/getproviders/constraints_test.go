package getproviders_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/getproviders"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProviderConstraints(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	testDir := helpers.TmpDirWOSymlinks(t)

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
	constraints, err := getproviders.ParseProviderConstraints(vfs.NewOSFS(), map[string]string{}, tfimpl.Terraform, testDir)
	require.NoError(t, err)

	assert.Equal(t, "~> 5.0", constraints["registry.terraform.io/hashicorp/aws"])
	assert.Equal(t, "~> 4.0", constraints["registry.terraform.io/cloudflare/cloudflare"])

	// Test parsing with OpenTofu implementation
	constraints, err = getproviders.ParseProviderConstraints(vfs.NewOSFS(), map[string]string{}, tfimpl.OpenTofu, testDir)
	require.NoError(t, err)

	assert.Equal(t, "~> 5.0", constraints["registry.opentofu.org/hashicorp/aws"])
	assert.Equal(t, "~> 4.0", constraints["registry.opentofu.org/cloudflare/cloudflare"])
}

func TestParseProviderConstraintsWithImplicitProvider(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	testDir := helpers.TmpDirWOSymlinks(t)

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
	constraints, err := getproviders.ParseProviderConstraints(vfs.NewOSFS(), map[string]string{}, tfimpl.Terraform, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints default to terraform registry and are normalized
	assert.Equal(t, "~> 5.0", constraints["registry.terraform.io/hashicorp/aws"])

	// Test parsing with OpenTofu implementation
	constraints, err = getproviders.ParseProviderConstraints(vfs.NewOSFS(), map[string]string{}, tfimpl.OpenTofu, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints default to OpenTofu registry and are normalized
	assert.Equal(t, "~> 5.0", constraints["registry.opentofu.org/hashicorp/aws"])
}

// TestParseProviderConstraintsWithShorthand pins that a shorthand entry and an object
// entry in the same block both reach the constraint map.
func TestParseProviderConstraintsWithShorthand(t *testing.T) {
	t.Parallel()

	testDir := venvtest.Root("/module")
	fsys := vfs.NewMemMapFS()

	terraformContent := `
terraform {
  required_providers {
    aws = ">= 5.0"
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
  }
}
`

	require.NoError(t, fsys.MkdirAll(testDir, 0o755))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(testDir, "main.tf"), []byte(terraformContent), 0o644))

	constraints, err := getproviders.ParseProviderConstraints(fsys, map[string]string{}, tfimpl.Terraform, testDir)
	require.NoError(t, err)

	assert.Equal(t, ">= 5.0.0", constraints["registry.terraform.io/hashicorp/aws"])
	assert.Equal(t, "~> 4.0", constraints["registry.terraform.io/cloudflare/cloudflare"])

	constraints, err = getproviders.ParseProviderConstraints(fsys, map[string]string{}, tfimpl.OpenTofu, testDir)
	require.NoError(t, err)

	assert.Equal(t, ">= 5.0.0", constraints["registry.opentofu.org/hashicorp/aws"])
	assert.Equal(t, "~> 4.0", constraints["registry.opentofu.org/cloudflare/cloudflare"])
}

// TestParseProviderConstraintsWithShorthandVariants pins the normalization of a
// shorthand constraint.
func TestParseProviderConstraintsWithShorthandVariants(t *testing.T) {
	t.Parallel()

	testDir := venvtest.Root("/module")
	fsys := vfs.NewMemMapFS()

	terraformContent := `
terraform {
  required_providers {
    aws      = "= 5.100.0"
    time     = "0.10"
    external = ">= 2.0, < 3.0"
  }
}
`

	require.NoError(t, fsys.MkdirAll(testDir, 0o755))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(testDir, "main.tf"), []byte(terraformContent), 0o644))

	constraints, err := getproviders.ParseProviderConstraints(fsys, map[string]string{}, tfimpl.OpenTofu, testDir)
	require.NoError(t, err)

	assert.Equal(t, "5.100.0", constraints["registry.opentofu.org/hashicorp/aws"])
	assert.Equal(t, "0.10.0", constraints["registry.opentofu.org/hashicorp/time"])
	assert.Equal(t, ">= 2.0.0, < 3.0.0", constraints["registry.opentofu.org/hashicorp/external"])
}

func TestParseProviderConstraintsWithEnvironmentOverride(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	testDir := helpers.TmpDirWOSymlinks(t)

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

	customRegistry := "custom.registry.example.com"
	env := map[string]string{"TG_TF_DEFAULT_REGISTRY_HOST": customRegistry}

	// Test parsing with Terraform implementation - should use custom registry
	constraints, err := getproviders.ParseProviderConstraints(vfs.NewOSFS(), env, tfimpl.Terraform, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints use custom registry for implicit providers and are normalized
	assert.Equal(t, "~> 5.0", constraints[customRegistry+"/hashicorp/aws"])
	// Explicit source should use custom registry too and be normalized
	assert.Equal(t, "~> 1.0", constraints[customRegistry+"/example/custom"])

	// Test parsing with OpenTofu implementation - should also use custom registry (environment override takes precedence)
	constraints, err = getproviders.ParseProviderConstraints(vfs.NewOSFS(), env, tfimpl.OpenTofu, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints use custom registry even with OpenTofu and are normalized
	assert.Equal(t, "~> 5.0", constraints[customRegistry+"/hashicorp/aws"])
	assert.Equal(t, "~> 1.0", constraints[customRegistry+"/example/custom"])
}

func TestParseProviderConstraintsWithTofuFiles(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	testDir := helpers.TmpDirWOSymlinks(t)

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
	constraints, err := getproviders.ParseProviderConstraints(vfs.NewOSFS(), map[string]string{}, tfimpl.OpenTofu, testDir)
	require.NoError(t, err)

	// Verify constraints from both .tf and .tofu files are parsed and normalized
	assert.Equal(t, "~> 5.0", constraints["registry.opentofu.org/hashicorp/aws"])
	assert.Equal(t, "~> 3.0", constraints["registry.opentofu.org/hashicorp/azurerm"])

	// Test parsing with Terraform implementation
	constraints, err = getproviders.ParseProviderConstraints(vfs.NewOSFS(), map[string]string{}, tfimpl.Terraform, testDir)
	require.NoError(t, err)

	// Verify constraints from both .tf and .tofu files are parsed with Terraform registry and normalized
	assert.Equal(t, "~> 5.0", constraints["registry.terraform.io/hashicorp/aws"])
	assert.Equal(t, "~> 3.0", constraints["registry.terraform.io/hashicorp/azurerm"])
}

// TestParseProviderConstraintsTofuWins pins which constraint survives when the
// same provider is declared in both a .tf and a .tofu file.
func TestParseProviderConstraintsTofuWins(t *testing.T) {
	t.Parallel()

	testDir := helpers.TmpDirWOSymlinks(t)

	declare := func(filename, version string) {
		content := `
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "` + version + `"
    }
  }
}
`
		require.NoError(t, os.WriteFile(filepath.Join(testDir, filename), []byte(content), 0644))
	}

	declare("main.tf", "~> 5.0")
	declare("main.tofu", "~> 6.0")

	constraints, err := getproviders.ParseProviderConstraints(
		vfs.NewOSFS(),
		map[string]string{},
		tfimpl.OpenTofu,
		testDir,
	)
	require.NoError(t, err)
	assert.Equal(t, "~> 6.0", constraints["registry.opentofu.org/hashicorp/aws"])
}

func TestParseProviderConstraintsWithEqualsPrefix(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	testDir := helpers.TmpDirWOSymlinks(t)

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
	constraints, err := getproviders.ParseProviderConstraints(vfs.NewOSFS(), map[string]string{}, tfimpl.Terraform, testDir)
	require.NoError(t, err)

	// Verify the parsed constraints are normalized (no "=" prefix)
	assert.Equal(t, "5.100.0", constraints["registry.terraform.io/hashicorp/aws"])
	assert.Equal(t, "4.40.0", constraints["registry.terraform.io/cloudflare/cloudflare"])
	assert.Equal(t, ">= 0.10.0", constraints["registry.terraform.io/hashicorp/time"])

	// Test parsing with OpenTofu implementation
	constraints, err = getproviders.ParseProviderConstraints(vfs.NewOSFS(), map[string]string{}, tfimpl.OpenTofu, testDir)
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
			name:     "pessimistic constraint keeps two parts",
			input:    "~> 4.0",
			expected: "~> 4.0",
		},
		{
			name:     "pessimistic constraint with a nonzero minor",
			input:    "~> 4.40",
			expected: "~> 4.40",
		},
		{
			name:     "pessimistic constraint with one part",
			input:    "~> 4",
			expected: "~> 4.0",
		},
		{
			name:     "pessimistic constraint with three zero parts",
			input:    "~> 4.0.0",
			expected: "~> 4.0.0",
		},
		{
			name:     "pessimistic constraint without a space",
			input:    "~>4.0",
			expected: "~> 4.0",
		},
		{
			name:     "pessimistic constraint with extra spaces",
			input:    "~>   4.0",
			expected: "~> 4.0",
		},
		{
			name:     "pessimistic constraint with a prerelease",
			input:    "~> 4.0-beta1",
			expected: "~> 4.0-beta1",
		},
		{
			name:     "pessimistic constraint with three parts and a prerelease",
			input:    "~> 4.0.1-beta1",
			expected: "~> 4.0.1-beta1",
		},
		{
			name:     "operator without a space",
			input:    ">=2.2",
			expected: ">= 2.2.0",
		},
		{
			name:     "unknown operator returned as-is",
			input:    "=> 2.2",
			expected: "=> 2.2",
		},
		{
			name:     "operator without a version returned as-is",
			input:    "~>",
			expected: "~>",
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
		{
			name:     "multi-part constraint normalizes each part",
			input:    ">= 3.0, < 7.0",
			expected: ">= 3.0.0, < 7.0.0",
		},
		{
			name:     "multi-part constraint with three parts",
			input:    ">= 2.0, >= 3.0, < 7.0",
			expected: ">= 2.0.0, >= 3.0.0, < 7.0.0",
		},
		{
			name:     "multi-part already normalized",
			input:    ">= 3.0.0, < 7.0.0",
			expected: ">= 3.0.0, < 7.0.0",
		},
		{
			name:     "multi-part with mixed operators",
			input:    "~> 5.0, != 5.3",
			expected: "~> 5.0, != 5.3.0",
		},
		{
			name:     "multi-part with a lower bound and a pessimistic constraint",
			input:    ">= 3.0, ~> 3.1",
			expected: ">= 3.0.0, ~> 3.1",
		},
		{
			name:     "multi-part with two pessimistic precisions",
			input:    "~> 3.1, ~> 3.1.4",
			expected: "~> 3.1, ~> 3.1.4",
		},
		{
			name:     "upper bound declared first",
			input:    "< 7.0, >= 3.0",
			expected: ">= 3.0.0, < 7.0.0",
		},
		{
			name:     "three terms declared out of order",
			input:    "!= 5.3, < 6, ~> 5.0",
			expected: "~> 5.0, != 5.3.0, < 6.0.0",
		},
		{
			name:     "repeated term",
			input:    ">= 3.0, >= 3.0.0",
			expected: ">= 3.0.0",
		},
		{
			name:     "repeated pessimistic term with one and two parts",
			input:    "~> 3, ~> 3.0",
			expected: "~> 3.0",
		},
		{
			name:     "repeated exact version with and without equals",
			input:    "= 3.2.3, 3.2.3",
			expected: "3.2.3",
		},
		{
			name:     "pessimistic precisions sharing a version",
			input:    "~> 3.2, ~> 3.2.0",
			expected: "~> 3.2.0, ~> 3.2",
		},
		{
			name:     "every operator sharing a version",
			input:    "!= 3.2.0, < 3.2.0, <= 3.2.0, ~> 3.2, ~> 3.2.0, 3.2.0, >= 3.2.0, > 3.2.0",
			expected: "> 3.2.0, >= 3.2.0, 3.2.0, ~> 3.2.0, ~> 3.2, <= 3.2.0, < 3.2.0, != 3.2.0",
		},
		{
			name:     "prerelease sorts before its release",
			input:    ">= 3.0.0, != 3.0.0-beta1",
			expected: "!= 3.0.0-beta1, >= 3.0.0",
		},
		{
			name:     "unrecognized term keeps the declared order",
			input:    "< 7.0, => 2.2, < 7.0",
			expected: "< 7.0.0, => 2.2, < 7.0.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// We need to call the unexported function through the public API
			// So we'll test it through the constraint parsing
			testDir := helpers.TmpDirWOSymlinks(t)
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

			constraints, err := getproviders.ParseProviderConstraints(vfs.NewOSFS(), map[string]string{}, tfimpl.Terraform, testDir)
			require.NoError(t, err)

			result := constraints["registry.terraform.io/example/test"]
			assert.Equal(t, tc.expected, result, "Input: %s", tc.input)
		})
	}
}
