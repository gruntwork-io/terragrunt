variable "name" {
  type = string
}

output "value" {
  value = "from-${var.name}"
}
