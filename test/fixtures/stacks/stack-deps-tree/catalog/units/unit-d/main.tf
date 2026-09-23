terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "marker" {
  content  = "unit-d"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = "from-d"
}
