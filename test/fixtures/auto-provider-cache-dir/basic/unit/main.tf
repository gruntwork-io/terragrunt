terraform {
  required_version = ">= 1.0"

  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "~> 3.0"
    }
  }
}
