include {
  path = find_in_parent_folders()
}

dependency "app1" {
  config_path = "../app1"

  mock_outputs = {
    app1_text = "(known after apply-all)"
  }
}

dependency "app3" {
  config_path = "../app3"

  mock_outputs = {
    app3_text = "(known after apply-all)"
  }
}

inputs = {
  app1_text = dependency.app1.outputs.app1_text
  app3_text = dependency.app3.outputs.app3_text
}
