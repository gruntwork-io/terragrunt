terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}

variable "token_via_input" {
  type = string
}

resource "local_file" "test" {
  filename = "./test-output.txt"
  content  = "Token via input: ${var.token_via_input}"
}
