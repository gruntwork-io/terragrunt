variable "vpc_id" {
  type    = string
  default = ""
}

output "nat_id" {
  value = "vpc-nat-${var.vpc_id}"
}
