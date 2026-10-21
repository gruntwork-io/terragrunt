# vnet/terragrunt.hcl
include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

locals {
  region = include.root.locals.region
}

terraform {
  source = "tfr:///Azure/avm-res-network-virtualnetwork/azurerm?version=0.22.3"
}

dependency "rg" {
  config_path = "../rg"

  mock_outputs = {
    resource_id = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/mock-rg"
  }

  mock_outputs_allowed_terraform_commands = ["plan"]
}

inputs = {
  name      = "my-vnet"
  location  = local.region
  parent_id = dependency.rg.outputs.resource_id

  address_space = ["10.0.0.0/16"]

  subnets = {
    private = {
      name             = "private"
      address_prefixes = ["10.0.1.0/24"]
    }
    public = {
      name             = "public"
      address_prefixes = ["10.0.101.0/24"]
    }
  }

  tags = {
    IaC         = "true"
    Environment = "dev"
  }
}
