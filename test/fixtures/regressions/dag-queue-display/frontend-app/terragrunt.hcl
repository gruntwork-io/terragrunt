dependency "backend" {
  config_path                             = "../backend-app"
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
  mock_outputs                            = {}
}
