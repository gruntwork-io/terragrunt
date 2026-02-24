locals {
  stack_dir = get_original_terragrunt_dir()
}

unit "unit_1" {
  source = find_in_parent_folders("units")
  path = "unit_1"

  values = {
    stack_dir = local.stack_dir
  }
}
