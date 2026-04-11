include {
  path = find_in_parent_folders("root.hcl")
}

iam_assume_role_with_existing_credentials = true

dependency "app1" {
  config_path = "../app1"

  mock_outputs = {
    app1_text = "(known after apply)"
  }
}

inputs = {
  app1_text = dependency.app1.outputs.app1_text
}
