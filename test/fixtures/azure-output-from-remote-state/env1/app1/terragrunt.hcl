include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "./"
}

dependency "app2" {
  config_path = "../app2"

  mock_outputs = {
    app2_output = "mock app2 output"
  }
}

dependency "app3" {
  config_path = "../app3"

  mock_outputs = {
    app3_output = "mock app3 output"
  }
}

inputs = {
  app2_output = dependency.app2.outputs.app2_output
  app3_output = dependency.app3.outputs.app3_output
}
