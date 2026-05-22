terraform {
  source = "."
}

include "remote_state" {
  path = find_in_parent_folders("_shared/remote_state.hcl")
}

dependency "sub" {
  config_path = "../sub"
}

dependency "rgs" {
  config_path = "../rgs"
}

inputs = {
  sub_id   = dependency.sub.outputs.id
  rgs_name = dependency.rgs.outputs.name
}
