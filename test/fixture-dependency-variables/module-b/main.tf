terraform {
  backend "local" {}
}

variable "content" {
  type = string
}

output "result" {
  value = "Hello World, from B: ${var.content}"
}
