terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "app1_output" {
  type = string
}

resource "local_file" "test" {
  content  = var.app1_output
  filename = "${path.module}/test.txt"
}
