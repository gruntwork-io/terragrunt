terraform {
  source = ".//"
}

include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "module_b" {
  config_path = "../module-b"
}

inputs = {
  test_var = dependency.module_b.outputs.test_output
}
