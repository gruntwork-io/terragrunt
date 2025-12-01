include "parent" {
  path = find_in_parent_folders("shared/parent-with-filters.hcl")
}

terraform {
  exclude_from_copy = ["child-exclude/**"]
  include_in_copy   = ["child-include.txt"]
}
