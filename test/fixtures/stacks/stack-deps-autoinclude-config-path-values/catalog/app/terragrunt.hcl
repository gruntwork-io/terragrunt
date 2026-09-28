terraform {
  source = "."
}

# The config_path references values.vpc_path, which the stack autoinclude overrides.
dependency "vpc" {
  config_path = values.vpc_path

  mock_outputs_allowed_terraform_commands = ["init", "plan", "validate"]
  mock_outputs = {
    vpc_id = "from-unit-mock"
  }
}

remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${dependency.vpc.outputs.vpc_id}.tfstate"
  }
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
