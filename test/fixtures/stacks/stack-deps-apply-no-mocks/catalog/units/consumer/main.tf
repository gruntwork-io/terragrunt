terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "producer_val" {
  type = string
}

resource "local_file" "marker" {
  content  = "consumer received: ${var.producer_val}"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = var.producer_val
}
