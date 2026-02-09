terraform {
  required_version = ">= 0.12"
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "~> 3.2.4"
    }
  }
}

resource "null_resource" "dummy" {}
