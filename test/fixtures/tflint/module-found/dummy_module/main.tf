terraform {
  required_version = ">= 0.12"
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = ">= 3.2.3"
    }
  }
}

resource "null_resource" "dummy" {}
