terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "5.36.0"
    }
    azure = {
      source  = "hashicorp/azurerm"
      version = "3.95.0"
    }
  }
}

module "naming" {
  source  = "cloudposse/label/null"
  version = "0.25.0"
}
