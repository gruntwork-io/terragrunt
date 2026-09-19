terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}


variable "value" {}

resource "local_file" "test" {
  content  = "test ${var.value}"
  filename = "${path.module}/test.txt"
}
