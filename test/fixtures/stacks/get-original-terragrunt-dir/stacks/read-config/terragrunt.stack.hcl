locals {
  common = read_terragrunt_config(find_in_parent_folders("live/common/stack_config.hcl"))
}

unit "unit_1" {
  source = find_in_parent_folders("units")
  path = "unit_1"

  values = merge({ stack_dir = local.common.locals.stack_dir }, try(values, {}))

  no_dot_terragrunt_stack = true
}

unit "unit_2" {
  source = find_in_parent_folders("units")
  path = "unit_2"

  values = merge({ stack_dir = local.common.locals.stack_dir }, try(values, {}))

  no_dot_terragrunt_stack = true
}
