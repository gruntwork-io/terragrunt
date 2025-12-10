
include "parent" {
  path = find_in_parent_folders("shared/parent.hcl")
}

terraform {
  exclude_from_copy = ["**/_*"]
  include_in_copy   = ["special-file.txt"]
}
