terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }

  backend "azurerm" {}
}

resource "null_resource" "test" {}
