terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "content" {}
variable "output_dir" {}

resource "local_file" "file" {
  content  = var.content
  filename = "${var.output_dir}/hi.txt"
} 