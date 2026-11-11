locals {
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))
}

inputs = {
  account = local.common.locals.account
  joined  = local.common.locals.joined
  split   = local.common.locals.split
}
