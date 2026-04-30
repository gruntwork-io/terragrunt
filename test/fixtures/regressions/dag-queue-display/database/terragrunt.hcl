dependency "vpc" {
  config_path                             = "../vpc"
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
  mock_outputs                            = {}
}
