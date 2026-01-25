terraform {
  required_providers {
    aws = {
      source  = "registry.opentofu.org/hashicorp/aws"
      version = "5.36.0"
    }
    azure = {
      source  = "registry.opentofu.org/hashicorp/azurerm"
      version = "3.95.0"
    }
  }
}

module "naming" {
  source  = "registry.opentofu.org/cloudposse/label/null"
  version = "0.25.0"
}
