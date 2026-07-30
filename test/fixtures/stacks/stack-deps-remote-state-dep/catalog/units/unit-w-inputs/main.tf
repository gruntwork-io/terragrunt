terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "input_marker" {
  content  = "input-marker"
  filename = "${path.module}/input.txt"
}
