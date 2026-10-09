dependency "dep" {
  config_path = "../dep"

  mock_outputs                            = { greeting = "mock" }
  mock_outputs_allowed_terraform_commands = ["plan", "init", "validate"]
}

terraform {
  source = "."
}

inputs = {
  msg = dependency.dep.outputs.greeting
}
