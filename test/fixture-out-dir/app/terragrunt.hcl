
dependency "dependency" {
  config_path = "../dependency"

  mock_outputs = {
    result = "46521694"
  }
  mock_outputs_allowed_terraform_commands = ["plan", "apply"]
}

inputs = {
  input_value = dependency.dependency.outputs.result
}