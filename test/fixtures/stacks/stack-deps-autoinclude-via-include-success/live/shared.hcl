unit "vpc" {
  source = "../catalog/units/vpc"
  path   = "vpc"
}

unit "subnet" {
  source = "../catalog/units/subnet"
  path   = "subnet"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan", "apply"]
      mock_outputs = {
        id = "shared-mock-id"
      }
    }

    inputs = {
      vpc_id = dependency.vpc.outputs.id
    }
  }
}
