terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}


resource "local_file" "file" {
  content  = "chick"
  filename = "${path.module}/test.txt"
}

output "output" {
  value = local_file.file.filename
}
