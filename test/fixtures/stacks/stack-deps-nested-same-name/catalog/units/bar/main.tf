variable "val" {
  type        = string
  description = "Value received from stack.foo.foo"
}

resource "local_file" "marker" {
  content  = "Received: ${var.val}"
  filename = "${path.module}/input.txt"
}

output "received_val" {
  value = var.val
}
