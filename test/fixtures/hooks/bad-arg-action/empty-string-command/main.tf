terraform {
  required_providers {
    external = {
      source  = "hashicorp/external"
      version = "2.3.3"
    }
  }
}

data "external" "example" {
  program = ["jq", "-n", "{\"example\": \"hello, world\"}"]
}

output "example" {
  value = data.external.example.result.example
}
