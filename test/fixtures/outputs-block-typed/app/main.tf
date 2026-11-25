variable "vpc_id" {
  type = string
}

variable "port" {
  type = string
}

variable "secret" {
  type = string
}

output "vpc_id" {
  value = var.vpc_id
}

output "port" {
  value = var.port
}

output "secret" {
  value = var.secret
}
