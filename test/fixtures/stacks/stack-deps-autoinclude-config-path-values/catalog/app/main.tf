variable "vpc_id" {
  type = string
}

output "app_vpc" {
  value = var.vpc_id
}
