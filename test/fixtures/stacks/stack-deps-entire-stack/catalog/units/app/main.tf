variable "vpc_id" {
  type = string
}

resource "local_file" "marker" {
  content  = "app on ${var.vpc_id}"
  filename = "${path.module}/marker.txt"
}
