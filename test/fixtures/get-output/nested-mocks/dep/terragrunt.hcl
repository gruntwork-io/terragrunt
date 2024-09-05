dependency "deepdep" {
  config_path = "../deepdep"
  mock_outputs = {
    output = "I am a mock"
  }
  mock_outputs_allowed_terraform_commands = ["validate"]
}

inputs = {
  input = dependency.deepdep.outputs.output
}
