module "vpc" {
  source = "../../../../fixture-preprocessor/modules/vpc"

  name             = var.vpc_config.name
  cidr_block       = var.vpc_config.cidr_block
  num_nat_gateways = var.vpc_config.num_nat_gateways
}