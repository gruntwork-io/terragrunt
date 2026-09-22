terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }
}

resource "null_resource" "db" {
  triggers = {
    name = "db"
  }
}

output "db_id" {
  value = "db-12345"
}
