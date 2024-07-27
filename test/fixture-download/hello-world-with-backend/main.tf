data "external" "test" {
  program = ["jq", "-n", "--arg", "name", var.name, "{\"test\": \"hello, \\($name)\"}"]
}

variable "name" {
  description = "Specify a name"
  type        = string
}

output "test" {
  value = data.external.test.result.test
}

terraform {
  # These settings will be filled in by Terragrunt
  backend "s3" {}

  required_providers {
    external = {
      source  = "hashicorp/external"
      version = "2.3.3"
    }
  }
}

