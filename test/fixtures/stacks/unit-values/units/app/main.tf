
variable "deployment" {}

variable "project" {}

variable "data" {}

output "data" {
  value = var.data
}

output "deployment" {
  value = var.deployment
}

output "project" {
  value = var.project
}