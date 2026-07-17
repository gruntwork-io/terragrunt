terraform {
  required_version = ">= 1.0"

  required_providers {
    null = {
      # Source is not fully qualified so the registry is resolved dynamically
      # based on the Terraform/OpenTofu implementation, allowing the provider
      # cache to intercept requests.
      source  = "hashicorp/null"
      version = "3.2.2"
    }
  }
}
