terraform {
  backend "local" {}
}

variable "unit" {
  type = string
}

output "unit" {
  value = var.unit
}
