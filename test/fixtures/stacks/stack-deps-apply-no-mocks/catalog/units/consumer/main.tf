variable "producer_val" {
  type = string
}

resource "local_file" "marker" {
  content  = "consumer received: ${var.producer_val}"
  filename = "${path.module}/marker.txt"
}

output "val" {
  value = var.producer_val
}
