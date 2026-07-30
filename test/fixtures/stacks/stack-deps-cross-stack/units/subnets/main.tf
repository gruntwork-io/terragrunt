terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "marker" {
  content  = "subnets"
  filename = "${path.module}/marker.txt"
}

output "subnet_id" {
  value = "subnet-cross-stack"
}
