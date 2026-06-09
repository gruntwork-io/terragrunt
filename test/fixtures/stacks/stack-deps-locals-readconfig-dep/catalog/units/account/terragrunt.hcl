locals {
  region_config = read_terragrunt_config(find_in_parent_folders("region.hcl"))
  region        = local.region_config.locals.region
  parent_marker = find_in_parent_folders("region.hcl")
}

terraform {
  source = "."
}

inputs = {
  account = values.account
  env     = values.env
  region  = local.region
  marker  = local.parent_marker
}
