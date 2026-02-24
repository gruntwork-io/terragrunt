locals {
  common = read_terragrunt_config(find_in_parent_folders("common/stack_config.hcl"))
}

stack "units" {
  source = find_in_parent_folders("stacks/no-locals")
  path   = "unit_dirs"

  values = {
    stack_dir = local.common.locals.stack_dir
  }
}
