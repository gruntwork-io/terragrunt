variable "dep_value" {
  type = string
}

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
  content  = "Dep had value: ${var.dep_value}"
  filename = "${path.module}/some_file.txt"
}

output "my_value" {
  value = local_file.some_file.content
}
