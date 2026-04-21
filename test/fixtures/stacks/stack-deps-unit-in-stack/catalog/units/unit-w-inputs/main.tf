variable "val" {
  type = string
}

resource "local_file" "marker" {
  content  = "received: ${var.val}"
  filename = "${path.module}/marker.txt"
}

output "received_val" {
  value = var.val
}
