include "root" {
  path = find_in_parent_folders("root.hcl")
}

inputs = { from_attr = "child" }

variables {
  kind   = "json"
  inputs = { from_block = "child" }
}
