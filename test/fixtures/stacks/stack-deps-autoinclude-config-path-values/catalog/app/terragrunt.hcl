terraform {
  source = "."
}

# The config_path references values.vpc_path, which the stack autoinclude will override.
# Without the fix, this fails with "Unsupported attribute" because values.vpc_path is evaluated
# before the autoinclude replacement is applied (issue 6692).
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
