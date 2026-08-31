dependency "dep" {
  config_path                             = "../dep"
  mock_outputs                            = { id = "mock" }
  mock_outputs_allowed_terraform_commands = ["exec"]
}

inputs = {
  id = dependency.dep.outputs.id
}
