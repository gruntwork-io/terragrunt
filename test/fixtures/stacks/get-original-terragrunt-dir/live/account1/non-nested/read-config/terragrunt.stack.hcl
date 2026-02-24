locals {
  common = read_terragrunt_config(find_in_parent_folders("common/stack_config.hcl"))
}

unit "unit_1" {
  source = find_in_parent_folders("units")
  path = "unit_1"

  values = {
    stack_dir = local.common.locals.stack_dir
  }
}
