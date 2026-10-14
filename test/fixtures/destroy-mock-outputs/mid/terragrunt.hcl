dependency "vpc" {
  config_path = "../vpc"

  mock_outputs_allowed_terraform_commands = ["destroy"]
  mock_outputs = {
    id = "mock-vpc"
  }
}

inputs = {
  vpc_id = dependency.vpc.outputs.id
}
