terraform {
  source = "."
}

dependency "ecr_cache" {
  config_path = try(values.dependency_path.ecr-cache, "")
  # force enabled so error manifests even when empty
  enabled     = true

  mock_outputs = {
    token = "mock-token"
  }

  mock_outputs_merge_strategy_with_state  = "shallow"
  mock_outputs_allowed_terraform_commands = ["init", "validate", "destroy", "plan"]
}

inputs = {
  token = dependency.ecr_cache.outputs.token
}


