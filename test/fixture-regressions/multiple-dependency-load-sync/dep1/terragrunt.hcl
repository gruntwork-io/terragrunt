include {
  path = find_in_parent_folders("root-terragrunt.hcl")
}

inputs = {
  name = "dep1"
}
