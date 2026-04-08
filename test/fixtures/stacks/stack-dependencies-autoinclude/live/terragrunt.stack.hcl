locals {
  env = "test"
}

unit "vpc" {
  source = "../../units/vpc"
  path   = "vpc"
}

unit "app" {
  source = "../../units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path

      mock_outputs_allowed_terraform_commands = ["plan"]
      mock_outputs = {
        vpc_id = "mock-vpc-id"
      }
    }

    inputs = {
      env    = local.env
      vpc_id = dependency.vpc.outputs.vpc_id
    }
  }
}
