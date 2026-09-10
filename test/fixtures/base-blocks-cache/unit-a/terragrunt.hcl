include "root" {
  path = find_in_parent_folders("root.hcl")
}

inputs = {
  unit = "unit-a"
}
