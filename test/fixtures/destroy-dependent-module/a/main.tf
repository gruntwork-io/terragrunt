terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "file" {
  content         = "module-a"
  filename        = "module-a.txt"
  file_permission = "0644"
}

output "value" {
  value = local_file.file.filename
}
