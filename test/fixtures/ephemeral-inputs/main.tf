terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "test" {
  type      = string
  default   = "test_value"
  ephemeral = true
}

output "output" {
  value = ephemeralasnull(var.test)
}

resource "local_file" "file" {
  content  = "test"
  filename = "${path.module}/test.txt"
}
