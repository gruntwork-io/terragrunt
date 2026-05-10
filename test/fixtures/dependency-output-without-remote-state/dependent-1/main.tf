variable "shared_value" {
  type = string
}

output "result" {
  value = var.shared_value
}
