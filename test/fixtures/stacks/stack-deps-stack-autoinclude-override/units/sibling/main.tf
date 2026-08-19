terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "ok" {
  type    = string
  default = ""
}

resource "local_file" "marker" {
  content  = "sibling"
  filename = "${path.module}/marker.txt"
}

output "origin" {
  value = "sibling"
}
