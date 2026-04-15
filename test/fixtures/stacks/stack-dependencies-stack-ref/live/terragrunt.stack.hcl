locals {
  env = "test"
}

# A stack containing vpc + subnets units
stack "networking" {
  source = "../catalog/stacks/networking"
  path   = "networking"
}

# OSS-3101: Unit that depends on the entire networking stack
unit "app_stack_dep" {
  source = "../units/app"
  path   = "app-stack-dep"

  autoinclude {
    dependency "networking" {
      config_path = stack.networking.path

      mock_outputs_allowed_terraform_commands = ["plan"]
      mock_outputs = {
        vpc = {
          vpc_id = "mock-vpc-id"
        }
      }
    }

    inputs = {
      env    = local.env
      vpc_id = dependency.networking.outputs.vpc.vpc_id
    }
  }
}

# OSS-3102: Unit that depends on a specific unit within the stack
unit "app_unit_in_stack" {
  source = "../units/app"
  path   = "app-unit-in-stack"

  autoinclude {
    dependency "vpc" {
      config_path = stack.networking.vpc.path

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
