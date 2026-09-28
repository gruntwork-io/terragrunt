terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "data" {}

resource "local_file" "file" {
  content         = "local file"
  filename        = "module-a-${var.data}.txt"
  file_permission = "0644"
}

output "value" {
  value = local_file.file.filename
}
