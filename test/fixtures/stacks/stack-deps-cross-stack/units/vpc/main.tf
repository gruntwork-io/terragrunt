terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "marker" {
  content  = "vpc"
  filename = "${path.module}/marker.txt"
}

output "vpc_id" {
  value = "vpc-cross-stack"
}
