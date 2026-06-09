variable "ok" {
  type    = string
  default = ""
}

resource "local_file" "marker" {
  content  = "sibling"
  filename = "${path.module}/marker.txt"
}

output "origin" {
  value = "sibling"
}
