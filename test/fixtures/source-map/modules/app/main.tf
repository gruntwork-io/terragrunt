variable "name" {}

variable "vpc_id" {}

output "app_url" {
  value = "https://${var.name}.${var.vpc_id}.foo.io"
}
