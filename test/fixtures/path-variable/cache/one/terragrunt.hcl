locals {
  env    = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  shared = read_terragrunt_config(find_in_parent_folders("shared.hcl"))
}

inputs = {
  env_unit    = local.env.locals.read_unit
  shared_read = local.shared.locals.read_file
}
