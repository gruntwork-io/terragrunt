dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    x = "mocked"
  }
  mock_outputs_allowed_terraform_commands = ["apply"]
}

inputs = {
  x = dependency.dep.outputs.x
}
