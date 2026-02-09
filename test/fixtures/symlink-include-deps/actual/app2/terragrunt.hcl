include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "app1" {
  config_path = "../app1"
}

terraform {
  source = "../../../module"
}

inputs = {
  name    = "app2"
  app1_id = dependency.app1.outputs.name
}
