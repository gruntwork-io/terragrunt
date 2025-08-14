terraform {
  # These settings will be filled in by Terragrunt
  backend "s3" {}

  required_version = ">= 1.5.7"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2"
    }
  }
}

variable "name" {
  description = "Specify a name"
  type        = string
}

resource "null_resource" "test" {
  provisioner "local-exec" {
    command = "echo 'hello, ${var.name}'"
  }
}

output "test" {
  value = "hello, ${var.name}"
}
