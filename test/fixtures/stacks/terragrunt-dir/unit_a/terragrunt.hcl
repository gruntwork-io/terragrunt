
include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "."
}

inputs = {
  terragrunt_dir = values.terragrunt_dir
}
