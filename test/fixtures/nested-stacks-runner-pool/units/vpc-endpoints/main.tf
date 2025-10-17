variable "vpc_id" {
  type    = string
  default = ""
}

output "endpoint_id" {
  value = "vpc-endpoint-${var.vpc_id}"
}
