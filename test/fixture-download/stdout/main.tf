terraform {
  required_providers {
    external = {
      source = "hashicorp/external"
      version = "2.3.3"
    }
  }
}

data "external" "foo" {
  program = ["jq", "-n", "{\"foo\": \"foo\"}"]
}

output "foo" {
  value = data.external.foo.result.foo
}
