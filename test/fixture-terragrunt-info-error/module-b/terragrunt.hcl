terraform {
  source = ".//"
}

dependency "module_a" {
  config_path                             = "../module-a"
  mock_outputs_allowed_terraform_commands = ["init", "plan", "validate"]
  mock_outputs_merge_strategy_with_state  = "shallow"
  mock_outputs = {
    test_mock = "abc"
  }
}

inputs = {
  test_var = dependency.module_a.outputs.test_mock
}