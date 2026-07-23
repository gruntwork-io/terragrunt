stack "networking" {
  source = "../stacks/networking"
  path   = "networking"
}

unit "app" {
  source = "../units/app"
  path   = "app"

  autoinclude {
    dependency "networking" {
      config_path = stack.networking.path

      mock_outputs = {
        vpc = {
          vpc_id = "mock-vpc-id"
        }
        subnets = {
          subnet_id = "mock-subnet-id"
        }
      }
    }

    inputs = {
      vpc_id    = dependency.networking.outputs.vpc.vpc_id
      subnet_id = dependency.networking.outputs.subnets.subnet_id
    }
  }
}
