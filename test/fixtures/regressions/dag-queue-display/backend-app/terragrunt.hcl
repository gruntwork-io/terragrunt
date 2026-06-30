dependency "database" {
  config_path                             = "../database"
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
  mock_outputs                            = {}
}
