# Child config that includes parent using default shallow merge
# and defines exclude_from_copy - this should NOT be dropped

include "parent" {
  path = find_in_parent_folders("shared/parent.hcl")
  # default merge_strategy = "shallow"
}

terraform {
  exclude_from_copy = ["**/_*"]
  include_in_copy   = ["special-file.txt"]
}