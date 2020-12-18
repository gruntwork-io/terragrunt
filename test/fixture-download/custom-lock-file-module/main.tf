# A dirt simple module for use with the custom-lock-file test

variable "name" {
  description = "The name to use"
  type        = string
}

output "text" {
  value = "Hello, ${var.name}"
}