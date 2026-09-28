terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
    aws = {
      source  = "example.com/hashicorp/aws"
      version = "5.59.0"
    }
    azurerm = {
      source  = "example.com/hashicorp/azurerm"
      version = "3.113.0"
    }
  }
}
