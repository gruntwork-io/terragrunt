include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "grandchild1" {
  config_path = "../grandchild1"
}
