variable "input" {
  default = null
}

output "output" {
  value = var.input == null ? "Hello, World" : ""
}
