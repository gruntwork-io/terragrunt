terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }
}

resource "null_resource" "vpc" {
  triggers = {
    name = "vpc"
  }
}

output "vpc_id" {
  value = "vpc-12345"
}

