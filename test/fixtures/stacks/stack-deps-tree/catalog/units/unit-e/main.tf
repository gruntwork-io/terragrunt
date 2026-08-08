terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}

resource "local_file" "marker" {
  content  = "unit-e"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = "from-e"
}
