
variable "project" {}

variable "env" {}

locals {
  config = "${var.project} ${var.env}"
}

resource "local_file" "file" {
  content  = local.config
  filename = "${path.module}/file.txt"
}

output "config" {
  value = local.config
}