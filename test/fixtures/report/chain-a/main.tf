terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }
}

resource "null_resource" "chain_a" {
  triggers = {
    always_fail = "true"
  }

  provisioner "local-exec" {
    command = "exit 1"
  }
}
