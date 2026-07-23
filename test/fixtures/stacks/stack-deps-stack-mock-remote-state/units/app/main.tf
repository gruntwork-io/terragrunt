terraform {
  backend "s3" {}
}

variable "vpc_id" {
  type = string
}

variable "subnet_id" {
  type = string
}

output "vpc_id" {
  value = var.vpc_id
}

output "subnet_id" {
  value = var.subnet_id
}
