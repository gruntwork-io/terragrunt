terraform {
  required_version = ">= 0.13"
}

variable "name" {
  description = "Name of the application"
  type        = string
}

output "name" {
  value = var.name
}
