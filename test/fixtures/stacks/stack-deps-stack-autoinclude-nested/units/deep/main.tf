terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "marker" {
  content  = "deep"
  filename = "${path.module}/marker.txt"
}

output "deep_id" {
  value = "deep-from-nested-autoinclude"
}
