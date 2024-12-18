terraform {
  source = ".//"
}

include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "module_a" {
  config_path = "../module-a"
}

inputs = {
  test_var = dependency.module_a.outputs.test_output
}
