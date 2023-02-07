variable "name" {
  description = "The name of the VPC"
  type        = string
}

variable "cidr_block" {
  description = "The CIDR block to use for the VPC"
  type        = string
}

variable "num_nat_gateways" {
  description = "The number of NAT Gateways to deploy in the VPC"
  type        = number
}