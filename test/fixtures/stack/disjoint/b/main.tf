terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }
}

resource "null_resource" "b" {}

output "b" {
  value = null_resource.b.id
}
