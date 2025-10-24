variable "first_value" {
  type = string
}

variable "second_value" {
  type = string
}

output "first_value" {
  value = var.first_value
}

output "second_value" {
  value = var.second_value
}
