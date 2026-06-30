terraform {
  source = "."
}

dependency "dep" {
  config_path = "../dep-with-feature"

  mock_outputs = {
    data = "mock"
  }
  mock_outputs_allowed_terraform_commands = ["plan", "validate"]
}

inputs = {
  dep_data = dependency.dep.outputs.data
}
