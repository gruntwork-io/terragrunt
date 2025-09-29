include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${find_in_parent_folders("catalog/modules")}//ddb"
}

inputs = {
  name = "best-cat-2025-09-24-2359-dev"
}
