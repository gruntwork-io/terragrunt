terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

data "local_file" "read_not_existing_file" {
  filename = "${path.module}/not-existing-file.txt"
}
