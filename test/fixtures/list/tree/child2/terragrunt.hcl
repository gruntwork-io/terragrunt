include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "grandchild2" {
  config_path = "../grandchild2"
}

