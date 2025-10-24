terraform {
  required_version = ">= 1.5.7"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2.4"
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
  source = "github.com/gruntwork-io/terragrunt.git//test/fixtures/download/hello-world-no-remote?ref=c574388b2f2821c10c77f0547570b57e32ef02a0"
  name   = var.name
}
