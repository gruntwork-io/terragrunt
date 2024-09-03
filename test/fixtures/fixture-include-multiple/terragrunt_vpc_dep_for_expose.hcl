dependency "vpc" {
  config_path = "${get_terragrunt_dir()}/../vpc"
  mock_outputs = {
    attribute     = "hello"
    old_attribute = "old val"
  }
  mock_outputs_allowed_terraform_commands = ["apply", "plan", "destroy", "output"]
}
