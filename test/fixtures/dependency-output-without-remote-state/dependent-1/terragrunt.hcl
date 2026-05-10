include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "shared" {
  config_path = "../shared"
}

terraform {
  source = ".//"
}

inputs = {
  shared_value = dependency.shared.outputs.value
}
