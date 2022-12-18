# Use a random resource as a mock VPC
resource "random_string" "mock_vpc" {
  keepers = {
    name             = var.name
    cidr_block       = var.cidr_block
    num_nat_gateways = var.num_nat_gateways
  }

  length  = 8
  numeric = false
  special = false
  upper   = false
}

locals {
  vpc_id = "vpc-${random_string.mock_vpc.id}"
}