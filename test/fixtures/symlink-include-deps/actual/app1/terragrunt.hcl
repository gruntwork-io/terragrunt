include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "vpc" {
  config_path = "../vpc"
}

terraform {
  source = "../../../module"
}

inputs = {
  name   = "app1"
  vpc_id = dependency.vpc.outputs.name
}
