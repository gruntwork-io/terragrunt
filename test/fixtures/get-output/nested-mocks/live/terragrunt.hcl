dependency "dep" {
  config_path = "../dep"
  mock_outputs = {
    output = "I am a shallow mock"
  }
  mock_outputs_allowed_terraform_commands = ["validate"]
}

inputs = {
  input = dependency.dep.outputs.output
}
