terraform {
  required_providers {
    external = {
      source  = "hashicorp/external"
      version = "2.3.3"
    }
  }
}

variable "person" {
  type = string
}

data "external" "example" {
  program = ["jq", "-n", "--arg", "person", var.person, "{\"example\": \"hello, \\($person)\"}"]
}

output "example" {
  value = data.external.example.result.example
}
