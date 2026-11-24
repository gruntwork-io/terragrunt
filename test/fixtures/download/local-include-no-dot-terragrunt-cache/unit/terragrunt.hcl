include "root" {
  path = find_in_parent_folders("root.hcl")
}

# This unit has its own terraform block; no_dot_terragrunt_cache still has to arrive from root.hcl.
terraform {
  copy_terraform_lock_file = true
}

inputs = {
  test_value = "include-no-dot-terragrunt-cache"
}
