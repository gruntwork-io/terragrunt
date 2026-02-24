terraform {
  required_version = ">= 0.12"
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = ">= 2.5.2"
    }
  }
}

variable "input" {
  description = "Project input"
  type        = string
}

resource "local_file" "file" {
  content  = var.input
  filename = "${path.module}/data.txt"
}

output "result" {
  value = var.input
}
