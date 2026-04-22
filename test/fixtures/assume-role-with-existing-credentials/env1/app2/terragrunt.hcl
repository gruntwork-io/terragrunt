include {
  path = find_in_parent_folders("root.hcl")
}

dependency "app1" {
  config_path = "../app1"

  mock_outputs = {
    app1_text = "(known after apply)"
  }
}

inputs = {
  app1_text = dependency.app1.outputs.app1_text
}
