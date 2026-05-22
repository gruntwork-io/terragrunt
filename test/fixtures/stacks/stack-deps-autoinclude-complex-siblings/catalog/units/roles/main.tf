variable "val" {
  type    = string
  default = "missing"
}

resource "local_file" "marker" {
  content  = "roles-received: ${var.val}"
  filename = "${path.module}/marker.txt"
}
