terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "val" {
  type    = string
  default = "missing"
}

resource "local_file" "marker" {
  content  = "roles-received: ${var.val}"
  filename = "${path.module}/marker.txt"
}
