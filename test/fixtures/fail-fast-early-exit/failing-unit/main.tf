terraform {
  required_version = ">= 1.0"

  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = ">= 3.2"
    }
  }
}

resource "null_resource" "fail" {
  provisioner "local-exec" {
    command = "echo 'failing-unit running' && exit 1"
  }
}
