terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "output_marker" {
  content  = "Hello from unit-w-outputs!"
  filename = "${path.module}/output.txt"
}

output "val" {
  value = "Hello!"
}
