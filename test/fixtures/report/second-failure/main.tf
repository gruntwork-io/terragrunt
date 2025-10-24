terraform {
  required_version = ">= 1.5.7"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2.4"
    }
  }
}

resource "null_resource" "test" {
  provisioner "local-exec" {
    command = "exit 1"
  }
}
