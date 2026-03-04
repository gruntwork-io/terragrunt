package tf_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModuleVariablesWithProviderFunctions verifies that ModuleVariables can parse
// HCL files that use provider function syntax (provider::namespace::function).
// This is a regression test for https://github.com/gruntwork-io/terragrunt/issues/3425
func TestModuleVariablesWithProviderFunctions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	hclContent := `
terraform {
  required_version = "~> 1.8"
  required_providers {
    assert = {
      source  = "hashicorp/assert"
      version = "~> 0.13"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.8"
    }
  }
}

data "azurerm_subnet" "main" {
  name                 = var.subnet.name
  resource_group_name  = var.subnet.resource_group_name
  virtual_network_name = var.subnet.virtual_network_name

  lifecycle {
    postcondition {
      condition     = var.subnet.enable_ipv6 == false || anytrue([ for prefix in self.address_prefixes: provider::assert::cidrv6(prefix) ])
      error_message = "Subnet does not contain valid IPv6 CIDR. Either use a subnet that contains a valid IPv6 CIDR or disable IPv6 support."
    }
  }
}

variable "subnet" {
  type = object({
    name                 = string
    virtual_network_name = string
    resource_group_name  = string
    enable_ipv6          = optional(bool, false)
  })
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(hclContent), 0644))

	required, optional, err := tf.ModuleVariables(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"subnet"}, required)
	assert.Empty(t, optional)
}
