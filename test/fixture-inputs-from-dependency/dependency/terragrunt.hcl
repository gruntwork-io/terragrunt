include "root" {
  path = find_in_parent_folders("terragrunt.hcl")
}

include "local" {
  path = "local.hcl"
}

inputs = {
  foo = "dependency-input-foo-value"
}
