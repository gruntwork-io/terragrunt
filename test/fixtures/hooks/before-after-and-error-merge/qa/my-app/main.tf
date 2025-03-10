terraform {
  backend "s3" {}

  required_version = ">= 1.5.7"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "3.2.3"
    }
  }
}

resource "null_resource" "example" {
  provisioner "local-exec" {
    command = "echo hello, world"
  }
}

output "example" {
  value = "hello, world"
}
