terraform {
  required_version = ">= 1.0"

  required_providers {
    local = {
      source  = "hashicorp/local"
      version = ">= 2.1"
    }
  }
}

resource "local_file" "some_file" {
  content  = "hello-from-dep"
  filename = "${path.module}/some_file.txt"
}

output "some_value" {
  value = local_file.some_file.content
}
