dependency "shared" {
  config_path = "../module2"

  # Mock outputs to avoid needing actual terraform state
  mock_outputs = {
    output_value = "mock"
  }
  mock_outputs_allowed_terraform_commands = ["plan", "destroy"]
}
