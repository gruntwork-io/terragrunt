include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

locals {
  region = include.root.locals.region
}

terraform {
  source = "tfr:///Azure/avm-res-resources-resourcegroup/azurerm?version=0.4.0"
}

inputs = {
  name     = "__FILL_IN_NAME_PREFIX__-step-five-rg-${local.region}"
  location = local.region

  tags = {
    IaC = "true"
    Environment = "dev"
  }
}
