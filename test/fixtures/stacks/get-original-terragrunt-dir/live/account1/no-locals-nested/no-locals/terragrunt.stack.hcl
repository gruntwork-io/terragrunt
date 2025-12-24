stack "units" {
  source = find_in_parent_folders("stacks/no-locals")
  path   = "unit_dirs"

  values = {
    stack_dir = get_original_terragrunt_dir()
  }
}
