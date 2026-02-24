terraform {
  required_version = ">= 1.5.0"

  required_providers {
    cloudflare = {
      # Source is not fully qualified so the registry is resolved dynamically
      # based on the Terraform/OpenTofu implementation, allowing the provider
      # cache to intercept requests.
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
    time = {
      # Source is not fully qualified so the registry is resolved dynamically
      # based on the Terraform/OpenTofu implementation, allowing the provider
      # cache to intercept requests.
      source  = "hashicorp/time"
      version = ">= 0.10.0"
    }
  }
}
