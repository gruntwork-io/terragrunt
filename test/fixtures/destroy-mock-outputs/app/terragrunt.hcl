dependency "mid" {
  config_path = "../mid"

  mock_outputs_allowed_terraform_commands = ["destroy"]
  mock_outputs = {
    id = "mock-mid"
  }
}

inputs = {
  mid_id = dependency.mid.outputs.id
}
