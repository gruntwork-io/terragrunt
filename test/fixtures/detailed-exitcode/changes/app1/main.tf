terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "example" {
  content  = "Test"
  filename = "${path.module}/example.txt"
}
