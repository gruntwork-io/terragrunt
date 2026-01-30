terraform {
  required_version = ">= 1.0"

  required_providers {
    aws = {
      # Not fully qualified to allow tests to run with both Terraform and OpenTofu
      # and verify that a different lock file will be generated for each.
      source  = "hashicorp/aws"
      version = "5.36.0"
    }
    azure = {
      # Not fully qualified to allow tests to run with both Terraform and OpenTofu
      # and verify that a different lock file will be generated for each.
      source  = "hashicorp/azurerm"
      version = "3.95.0"
    }
  }
}
