variable "vpc_config" {
  description = "The config for the VPC"
  type = object({
    name             = string
    cidr_block       = string
    num_nat_gateways = number
  })
}