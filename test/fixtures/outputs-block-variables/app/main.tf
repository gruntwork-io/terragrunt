variable "vpc_id" {
  type = string
}

variable "subnet_ids" {
  type = list(string)
}

output "vpc_id" {
  value = var.vpc_id
}

output "subnet_ids" {
  value = var.subnet_ids
}
