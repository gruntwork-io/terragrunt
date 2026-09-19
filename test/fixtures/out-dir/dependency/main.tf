terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "file" {
  content  = "dependency file"
  filename = "${path.module}/dependency_file.txt"
}

output "result" {

  value = "42"
}