# VPC Module

This Terraform Module creates a production-ready AWS Virtual Private Cloud (VPC) with public and private subnets, NAT gateways, and Internet gateway for secure network isolation.

## How does this work?

This module provisions a standard three-tier VPC architecture:

- Creates a VPC with configurable CIDR block
- Provisions public subnets for internet-facing resources
- Provisions private subnets for internal resources
- Deploys NAT gateways for outbound internet access from private subnets
- Configures route tables and subnet associations
- Optionally enables VPC Flow Logs

## How do you use this module?
```hcl
module "vpc" {
  source = "git::https://github.com/example-org/infrastructure-modules.git//infrastructure/terraform-aws-vpc"

  vpc_name             = "production"
  cidr_block           = "10.0.0.0/16"
  availability_zones   = ["us-east-1a", "us-east-1b"]
  
  public_subnet_cidrs  = ["10.0.1.0/24", "10.0.2.0/24"]
  private_subnet_cidrs = ["10.0.10.0/24", "10.0.11.0/24"]
  
  enable_nat_gateway = true
  enable_dns_support = true
  
  tags = {
    Environment = "production"
  }
}
```

## What resources does this module create?

- VPC
- Internet Gateway
- NAT Gateways
- Public and Private Subnets
- Route Tables
- Route Table Associations
