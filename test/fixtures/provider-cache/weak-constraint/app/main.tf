terraform {
  required_version = ">= 1.5.0"

  required_providers {
    cloudflare = {
      source  = "registry.opentofu.org/cloudflare/cloudflare"
      version = "~> 4.0"
    }
    time = {
      source  = "registry.opentofu.org/hashicorp/time"
      version = ">= 0.10.0"
    }
  }
}
