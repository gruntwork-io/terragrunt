terraform {
  required_version = ">= 1.5.7"
}

variable "unit_a_required" {
  type = string
}

output "unit_a_required" {
  value = var.unit_a_required
}
