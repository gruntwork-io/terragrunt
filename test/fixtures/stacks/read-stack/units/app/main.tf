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

variable "stack_local_project" {}

variable "unit_source" {}

variable "unit_name" {}

variable "unit_value_version" {}

variable "stack_source" {}

variable "stack_value_env" {}

output "stack_local_project" {
  value = var.stack_local_project
}

output "unit_source" {
  value = var.unit_source
}

output "unit_name" {
  value = var.unit_name
}

output "unit_value_version" {
  value = var.unit_value_version
}

output "stack_source" {
  value = var.stack_source
}

output "stack_value_env" {
  value = var.stack_value_env
}