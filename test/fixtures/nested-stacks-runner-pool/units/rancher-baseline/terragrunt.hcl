terraform {
  source = "."
}

# Dependencies from which we may or may not consume outputs
dependencies {
  paths = try(values.dependencies, [])
}

dependency "rancher-bootstrap" {
  config_path = try(values.dependency_path.rancher-bootstrap, "../rancher-bootstrap")

  mock_outputs = {
    bootstrap_status = "mock-bootstrap-status"
  }

  mock_outputs_merge_strategy_with_state  = "shallow"
  mock_outputs_allowed_terraform_commands = ["init", "validate", "destroy"]
}

inputs = {
  bootstrap_status = dependency.rancher-bootstrap.outputs.bootstrap_status
}
