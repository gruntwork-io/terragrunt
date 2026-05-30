variable "val" {
  type = string
}

resource "local_file" "marker" {
  content  = "consumer received: ${var.val}"
  filename = "${path.module}/input.txt"
}

output "received" {
  value = var.val
}
