terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}


resource "local_file" "config" {
  content  = "${var.project_name} vpc: ${var.vpc} replica_count: ${var.replica_count}"
  filename = "config.txt"
}
