variable "project" {
  description = "The project identifier"
  type        = string
}

variable "env" {
  description = "The environment name"
  type        = string
}

variable "data" {
  description = "Additional data for the configuration"
  type        = string
}

locals {
  config = "${var.project} ${var.env} ${var.data}"
}

resource "local_file" "file" {
  content  = local.config
  filename = "${path.module}/file.txt"
}

output "config" {
  value       = local.config
  description = "The combined configuration string"
}

output "project" {
  value       = var.project
  description = "The project identifier"
}

output "env" {
  value       = var.env
  description = "The environment name"
}

output "data" {
  value       = var.data
  description = "Additional data used in the configuration"
}
