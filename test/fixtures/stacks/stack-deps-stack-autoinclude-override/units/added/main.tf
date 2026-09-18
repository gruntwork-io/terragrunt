terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "marker" {
  content  = "added"
  filename = "${path.module}/marker.txt"
}

output "origin" {
  value = "added"
}
