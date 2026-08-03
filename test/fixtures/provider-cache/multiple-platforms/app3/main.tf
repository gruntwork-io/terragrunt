terraform {
  required_version = ">= 1.0"
  required_providers {
    null = {
      # Not fully qualified to allow tests to run with both Terraform and OpenTofu
      # and verify that a different lock file will be generated for each.
      source  = "hashicorp/null"
      version = "3.2.3"
    }
    local = {
      # Not fully qualified to allow tests to run with both Terraform and OpenTofu
      # and verify that a different lock file will be generated for each.
      source  = "hashicorp/local"
      version = "2.5.2"
    }
  }
}
