include "root" {
  path = find_in_parent_folders("root.hcl")
}

# root.hcl also has a terraform block, so the include merges attribute by attribute;
# no_dot_terragrunt_cache set here must survive that merge.
terraform {
  no_dot_terragrunt_cache = true
}

inputs = {
  test_value = "include-no-dot-terragrunt-cache-child"
}
