terraform {
  required_version = ">= 1.5.7"
}

variable "direct_required" {
  type = string
}

output "direct_required" {
  value = var.direct_required
}
