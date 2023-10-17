terraform {
  backend "local" {}
}

variable "content" {
  type = string
}

output "result" {
  value = "Hello World, from A: ${var.content}"
}
