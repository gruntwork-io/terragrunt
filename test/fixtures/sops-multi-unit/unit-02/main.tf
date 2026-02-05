variable "secret_value" {
  type = string
}

variable "unit_name" {
  type = string
}

output "secret_value" {
  value = var.secret_value
}

output "unit_name" {
  value = var.unit_name
}
