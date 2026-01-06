unit "unit_1" {
  source = find_in_parent_folders("units")
  path = "unit_1"

  values = try(values, {})

  no_dot_terragrunt_stack = true
}

unit "unit_2" {
  source = find_in_parent_folders("units")
  path = "unit_2"

  values = try(values, {})

  no_dot_terragrunt_stack = true
}
