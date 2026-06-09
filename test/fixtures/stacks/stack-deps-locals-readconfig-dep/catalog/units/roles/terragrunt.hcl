locals {
  region_config = read_terragrunt_config(find_in_parent_folders("region.hcl"))
  region        = local.region_config.locals.region
}

terraform {
  source = "."
}

inputs = {
  roles  = values.roles
  env    = values.env
  region = local.region
}
