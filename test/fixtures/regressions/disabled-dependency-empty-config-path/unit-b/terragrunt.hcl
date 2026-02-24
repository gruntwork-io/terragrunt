include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../modules/id"
}

locals {
  # Test case: disabled dependency with empty config_path
  # This should NOT cause cycle errors - the empty path should be ignored
  # because the dependency is disabled
  unit_a_path = ""
}

dependency "unit_a" {
  config_path = try(local.unit_a_path, "")

  enabled = false

  mock_outputs = {
    random_string = ""
  }

  mock_outputs_merge_strategy_with_state  = "shallow"
  mock_outputs_allowed_terraform_commands = ["init", "validate", "destroy"]
}

inputs = {
  suffix = try(dependency.unit_a.outputs.random_string, "")
}
