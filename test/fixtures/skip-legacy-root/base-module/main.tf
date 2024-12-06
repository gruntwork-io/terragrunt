terraform {
  required_version = ">= 0.12"
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.5.2"
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
