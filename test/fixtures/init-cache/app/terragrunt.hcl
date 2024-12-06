include "root" {
  path   = find_in_parent_folders("root.hcl")
}

inputs = {
  env = "nonprod"
}

