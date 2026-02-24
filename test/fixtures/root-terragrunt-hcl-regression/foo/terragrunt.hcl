include "root" {
  // This is deprecated behavior, but we want to test that it still works.
  path = find_in_parent_folders("terragrunt.hcl")
}

terraform {
  source = "."
}
