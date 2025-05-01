include {
  path = find_in_parent_folders("root.hcl")
}

dependency "app1" {
  config_path = "../app1"

  mock_outputs = {
    app1_text = "(known after run --all apply)"
  }
}

dependency "app3" {
  config_path = "../app3"

  mock_outputs = {
    app3_text = "(known after run --all apply)"
  }
}

inputs = {
  app1_text = dependency.app1.outputs.app1_text
  app3_text = dependency.app3.outputs.app3_text
}
