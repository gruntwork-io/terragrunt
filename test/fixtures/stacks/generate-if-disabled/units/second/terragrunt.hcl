dependency "first" {
  config_path = find_in_parent_folders(".terragrunt-stack/first")
  mock_outputs = {
    test_output = "mock Hello, World!"
  }
}

inputs = {
  test_output = dependency.first.outputs.test_output
}
