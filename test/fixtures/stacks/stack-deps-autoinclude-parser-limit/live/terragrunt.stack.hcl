// Stack file uses format() in path: production parser handles it, simplified two-pass parser cannot. With autoinclude declared, generation must fail loudly.

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = format("%s", "vpc")
}

unit "subnet" {
  source = "../catalog/units/subnet"
  path   = "subnet"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan", "apply"]
      mock_outputs = {
        id = "mock-id"
      }
    }

    inputs = {
      vpc_id = dependency.vpc.outputs.id
    }
  }
}
