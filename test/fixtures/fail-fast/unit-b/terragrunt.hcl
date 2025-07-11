dependency "unita" {
  config_path = "../unit-a"
  mock_outputs_allowed_terraform_commands = ["validate"]
  mock_outputs = {
    data = "test-data"
  }
}

inputs = {
    data = dependency.unita.outputs.data
}