dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    x = "mocked"
  }
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
}

inputs = {
  x = dependency.dep.outputs.x
}
