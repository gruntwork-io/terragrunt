variable "vpc_id" {
  type = string
}

output "id" {
  value = "mid-${var.vpc_id}"
}
