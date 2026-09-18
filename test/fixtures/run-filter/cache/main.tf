terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }
}

variable "vpc_id" {
  type = string
}

resource "null_resource" "cache" {
  triggers = {
    name   = "cache"
    vpc_id = var.vpc_id
  }
}

output "cache_id" {
  value = "cache-${var.vpc_id}"
}

