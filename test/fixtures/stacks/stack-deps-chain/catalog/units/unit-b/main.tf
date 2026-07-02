variable "val_from_c" {
  type = string
}

resource "local_file" "marker" {
  content  = "unit-b received: ${var.val_from_c}"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = "from-b(${var.val_from_c})"
}
