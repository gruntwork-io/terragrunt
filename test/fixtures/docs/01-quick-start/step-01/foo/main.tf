terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "file" {
  content  = "Hello, World!"
  filename = "${path.module}/hi.txt"
}
