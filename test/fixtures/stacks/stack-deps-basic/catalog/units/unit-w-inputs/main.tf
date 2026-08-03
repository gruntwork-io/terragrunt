terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "val" {
  type        = string
  description = "Value received from the dependency unit"
}

resource "local_file" "input_marker" {
  content  = "Received: ${var.val}"
  filename = "${path.module}/input.txt"
}

output "received_val" {
  value = var.val
}
