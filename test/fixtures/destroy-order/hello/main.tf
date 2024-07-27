terraform {
  required_providers {
    external = {
      source  = "hashicorp/external"
      version = "2.3.3"
    }
  }
}

data "external" "test" {
  program = ["jq", "-n", "--arg", "module", module.hello.hello, "--arg", "name", var.name, "{\"test\": \"\\($module), \\($name)\"}"]
}

variable "name" {
  description = "Specify a name"
  type        = string
}

output "test" {
  value = data.external.test.result.test
}

module "hello" {
  source = "./hello"
}
