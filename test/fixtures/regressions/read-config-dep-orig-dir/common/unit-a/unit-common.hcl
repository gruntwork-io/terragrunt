dependency "b" {
  config_path = format("%s/../unit-b", get_original_terragrunt_dir())

  mock_outputs = {
    name      = ""
    dep_value = ""
  }
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
}

inputs = {
  from_dep = dependency.b.outputs.name
}
