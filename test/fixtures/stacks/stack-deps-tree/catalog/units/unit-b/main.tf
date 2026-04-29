variable "val_from_d" {
  type = string
}

variable "val_from_e" {
  type = string
}

resource "local_file" "marker" {
  content  = "unit-b(${var.val_from_d},${var.val_from_e})"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = "from-b(${var.val_from_d},${var.val_from_e})"
}
