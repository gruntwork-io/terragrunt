terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "val_from_b" {
  type = string
}

variable "val_from_c" {
  type = string
}

resource "local_file" "marker" {
  content  = "unit-a(${var.val_from_b},${var.val_from_c})"
  filename = "${path.module}/marker.txt"
}
