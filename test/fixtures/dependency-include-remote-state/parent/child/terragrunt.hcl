terraform {
  source = "."
}

include "remote_state" {
  path = find_in_parent_folders("_shared/remote_state.hcl")
}

dependency "parent" {
  config_path = "../"
}

dependency "sub" {
  config_path = "../../sub"
}

dependency "rgs" {
  config_path = "../../rgs"
}

inputs = {
  parent_value = dependency.parent.outputs.value
}
