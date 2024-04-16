skip = true

terraform {
  source = "../module"
}

dependency "first" {
  config_path = "../first"

  mock_outputs_allowed_terraform_commands = ["init", "destroy", "validate"]
  mock_outputs_merge_strategy_with_state  = "deep_map_only"
  mock_outputs = {
    random_output = ""
  }
}

inputs = {
  input = dependency.first.outputs.random_output
}