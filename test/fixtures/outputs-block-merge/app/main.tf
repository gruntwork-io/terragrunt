variable "vpc_id" {
  type = string
}

variable "nat_id" {
  type = string
}

output "vpc_id" {
  value = var.vpc_id
}

output "nat_id" {
  value = var.nat_id
}
