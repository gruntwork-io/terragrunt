variable "vpc_id" {
  type = string
}

output "status" {
  value = "running-on-${var.vpc_id}"
}
