terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "marker" {
  content  = "extra"
  filename = "${path.module}/marker.txt"
}

output "extra_id" {
  value = "extra-from-stack-autoinclude"
}
