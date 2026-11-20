variable "vpc_id" {
  type = string
}

variable "nat_id" {
  type = string
}

output "vpc_id" {
  value = var.vpc_id
}

output "nat_is_null" {
  value = var.nat_id == null
}
