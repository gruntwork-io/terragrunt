terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "~> 3.0"
    }
  }
}

resource "null_resource" "main" {}

output "id" {
  value = null_resource.main.id
}
