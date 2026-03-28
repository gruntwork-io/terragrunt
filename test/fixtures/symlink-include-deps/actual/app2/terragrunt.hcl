include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "app1" {
  config_path = "../app1"
  mock_outputs = {
    name = "mock-app1"
  }
  mock_outputs_allowed_terraform_commands = ["validate"]
}

terraform {
  source = "../../module"
}

inputs = {
  name    = "app2"
  app1_id = dependency.app1.outputs.name
}
