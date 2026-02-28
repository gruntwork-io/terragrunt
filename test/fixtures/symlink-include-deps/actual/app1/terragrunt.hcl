include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "vpc" {
  config_path = "../vpc"
  mock_outputs = {
    name = "mock-vpc"
  }
  mock_outputs_allowed_terraform_commands = ["validate"]
}

terraform {
  source = "../../module"
}

inputs = {
  name   = "app1"
  vpc_id = dependency.vpc.outputs.name
}
