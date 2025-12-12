locals {
  stack_dir = get_original_terragrunt_dir()
}

stack "units" {
  source = find_in_parent_folders("stacks/with-locals")
  path   = "unit_dirs"

  values = {
    stack_dir = local.stack_dir
  }
}
