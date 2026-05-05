include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "./"
}

dependency "app3" {
  config_path = "../app3"

  mock_outputs = {
    app3_output = "app3 output"
  }
}

inputs = {
  app3_output = dependency.app3.outputs.app3_output
}
