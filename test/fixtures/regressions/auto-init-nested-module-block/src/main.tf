terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "~> 3.2"
    }
  }
}

provider "null" {}

resource "null_resource" "noop" {}
