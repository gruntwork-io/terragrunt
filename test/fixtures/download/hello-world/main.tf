terraform {
  required_version = ">= 1.5.7"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "3.2.3"
    }
  }
}

variable "name" {
  description = "Specify a name"
  type        = string
}

module "hello" {
  source = "./hello"
}

resource "null_resource" "test" {
  provisioner "local-exec" {
    command = "echo '${module.hello.hello}, ${var.name}'"
  }
}

output "test" {
  value = "${module.hello.hello}, ${var.name}"
}

module "remote" {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixture-download/hello-world?ref=v0.9.9"
  name   = var.name
}
