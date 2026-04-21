# Child config that uses include to find root.hcl
include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../module"
}
