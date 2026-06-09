terraform {
  required_version = ">= 1.5.7"
}

variable "unit_b_required" {
  type = string
}

output "unit_b_required" {
  value = var.unit_b_required
}
