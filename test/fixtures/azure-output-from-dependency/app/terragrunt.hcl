include "common" {
  path = find_in_parent_folders("common.hcl")
}

dependencies {
  paths = ["../dependency"]
}

dependency "test" {
  config_path = "../dependency"
}

inputs = {
  vpc_config = dependency.test.outputs
}
