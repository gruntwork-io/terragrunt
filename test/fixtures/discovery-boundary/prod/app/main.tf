terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }
}

variable "db_id" {
  type = string
}

variable "dns_id" {
  type = string
}

resource "null_resource" "app" {
  triggers = {
    name   = "app"
    db_id  = var.db_id
    dns_id = var.dns_id
  }
}

output "app_id" {
  value = "app-${var.db_id}-${var.dns_id}"
}
