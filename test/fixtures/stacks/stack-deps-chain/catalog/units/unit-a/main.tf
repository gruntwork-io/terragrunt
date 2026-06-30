variable "val_from_b" {
  type = string
}

resource "local_file" "marker" {
  content  = "unit-a received: ${var.val_from_b}"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = "from-a(${var.val_from_b})"
}
