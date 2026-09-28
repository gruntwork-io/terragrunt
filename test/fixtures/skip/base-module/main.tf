terraform {
  required_version = ">= 0.12"
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "person" {
  type = string
}

resource "local_file" "example" {
  content  = "hello, ${var.person}"
  filename = "example.txt"
}

output "example" {
  value = local_file.example.content
}
