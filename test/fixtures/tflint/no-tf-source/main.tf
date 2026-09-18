terraform {
  required_providers {
    random = {
      source  = "registry.opentofu.org/hashicorp/random"
      version = "3.8.0"
    }
  }

  required_version = ">= 1.2.7"
}

resource "random_id" "env" {
  byte_length = 8
}
