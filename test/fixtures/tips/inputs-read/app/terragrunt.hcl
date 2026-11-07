include "root" {
  path = find_in_parent_folders("root.hcl")
}

inputs = {
  message = yamldecode(file("../config.yml")).message
}
