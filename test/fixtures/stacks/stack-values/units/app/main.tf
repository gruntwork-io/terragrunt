
variable "project" {}

variable "env" {}

variable "data" {}

locals {
  config = "${var.project} ${var.env} ${var.data}"
}

resource "local_file" "file" {
  content  = local.config
  filename = "${path.module}/file.txt"
}

output "config" {
  value = local.config
}

output "project" {
  value = var.project
}

output "env" {
  value = var.env
}

output "data" {
  value = var.data
}
