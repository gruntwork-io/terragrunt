terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "vpc_id" {
  type = string
}

resource "local_file" "marker" {
  content  = "app received: ${var.vpc_id}"
  filename = "${path.module}/marker.txt"
}

output "status" {
  value = "running-on-${var.vpc_id}"
}
