terraform {
  required_providers {
    aws = {
      source  = "registry.opentofu.org/hashicorp/aws"
      version = "5.40.0"
    }
    azure = {
      source  = "registry.opentofu.org/hashicorp/azurerm"
      version = "3.95.0"
    }
    kubernetes = {
      source  = "registry.opentofu.org/hashicorp/kubernetes"
      version = "2.27.0"
    }
  }
}
