unit "unit_1" {
  source = find_in_parent_folders("units")
  path = "unit_1"

  values = {
    stack_dir = get_original_terragrunt_dir()
  }
}
