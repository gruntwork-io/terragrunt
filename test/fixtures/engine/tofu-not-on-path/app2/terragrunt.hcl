include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "app1" {
  config_path = "../app1"
}

inputs = {
  app1_output = dependency.app1.outputs.value
}
