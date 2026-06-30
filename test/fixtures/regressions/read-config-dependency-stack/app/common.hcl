dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    name = "mock-dep-output"
  }
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
}

inputs = {
  dep_name = dependency.dep.outputs.name
}
