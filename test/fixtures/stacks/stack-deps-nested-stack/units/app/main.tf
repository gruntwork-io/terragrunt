variable "val" {
  type = string
}

resource "local_file" "marker" {
  content  = "app received: ${var.val}"
  filename = "${path.module}/marker.txt"
}
