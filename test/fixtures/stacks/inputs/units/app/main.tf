terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "content" {
  type = string
}

variable "filename" {
  type    = string
  default = "file.txt"
}

resource "local_file" "file" {
  content  = var.content
  filename = "${path.module}/${var.filename}"
}

output "output" {
  value = local_file.file.filename
}
