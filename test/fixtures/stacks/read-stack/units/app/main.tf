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

variable "stack_local_project" {
  description = "The local project identifier for the stack configuration"
  type        = string
}

variable "unit_source" {
  description = "The source location for the unit configuration"
  type        = string
}

variable "unit_name" {
  description = "The name assigned to the unit"
  type        = string
}

variable "unit_value_version" {
  description = "The version identifier for the unit value"
  type        = string
}

variable "stack_source" {
  description = "The source location for the stack configuration"
  type        = string
}

variable "stack_value_env" {
  description = "The environment setting for the stack values"
  type        = string
}
output "stack_local_project" {
  value       = var.stack_local_project
  description = "The local project identifier from the stack configuration"
}

output "unit_source" {
  value       = var.unit_source
  description = "The source location for the unit"
}

output "unit_name" {
  value       = var.unit_name
  description = "The name identifier of the unit"
}

output "unit_value_version" {
  value       = var.unit_value_version
  description = "The version of the unit's value configuration"
}

output "stack_source" {
  value       = var.stack_source
  description = "The source location for the stack"
}

output "stack_value_env" {
  value       = var.stack_value_env
  description = "The environment configuration value for the stack"
}