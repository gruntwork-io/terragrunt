# Simple unit with a dependency on a shared module (module2)
# When running run --all from here, only bastion should be in the RUN queue
# The external dependency (module2) should be resolved for outputs but NOT executed
dependency "shared" {
  config_path = "../module2"

  # Mock outputs to avoid needing actual terraform state
  mock_outputs = {
    output_value = "mock"
  }
  mock_outputs_allowed_terraform_commands = ["plan", "destroy"]
}
