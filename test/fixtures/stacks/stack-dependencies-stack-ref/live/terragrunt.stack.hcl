locals {
  env = "test"
}

# A stack containing vpc + subnets units
stack "networking" {
  source = "../catalog/stacks/networking"
  path   = "networking"
}

# Unit that depends on the entire networking stack
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
