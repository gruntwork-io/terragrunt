# Common configuration that uses deprecated dependency.*.inputs.* syntax
# This should trigger the bug when included with expose = true

dependency "dep" {
  config_path = "../dep"
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
  mock_outputs = {
    some_value = "mock-value"
  }
  mock_outputs_merge_strategy_with_state = "shallow"
}

# Using deprecated syntax - this should be caught but isn't in partial parse
inputs = {
  value_from_dep = dependency.dep.inputs.some_value
}
