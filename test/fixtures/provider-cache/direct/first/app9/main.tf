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
