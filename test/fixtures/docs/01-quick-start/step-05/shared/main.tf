terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "content" {}

resource "local_file" "file" {
  content  = var.content
  filename = "${path.module}/hi.txt"
}
