# Child config that includes parent with filters using default shallow merge
# Child values should completely override parent values (not concatenate)

include "parent" {
  path = find_in_parent_folders("shared/parent-with-filters.hcl")
  # default merge_strategy = "shallow"
}

terraform {
  exclude_from_copy = ["child-exclude/**"]
  include_in_copy   = ["child-include.txt"]
}