terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "test_file" {
  content  = "test_file"
  filename = "${path.module}/test_file.txt"
}

