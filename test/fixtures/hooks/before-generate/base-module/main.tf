terraform {
  required_version = ">= 1.5.7"

  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "~> 3.2"
    }
  }
}

resource "null_resource" "example" {}

output "example" {
  value = "hello, world"
}
