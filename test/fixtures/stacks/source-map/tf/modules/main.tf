

variable "input" {
  description = "Project input"
  type        = string
}

resource "local_file" "file" {
  content  = var.input
  filename = "${path.module}/data.txt"
}

output "result" {
  value = var.input
}
