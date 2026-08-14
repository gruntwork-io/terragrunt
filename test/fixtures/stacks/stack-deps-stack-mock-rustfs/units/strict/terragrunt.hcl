include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "."
}

# The outputs are deliberately unreferenced: with mocks limited to validate, plan has to fail on the
# stack unit that has no state, rather than on an input that can't be evaluated.
dependency "networking" {
  config_path = "../networking"

  mock_outputs = {
    vpc = {
      id = "mock-vpc-id"
    }
    subnets = {
      id = "mock-subnet-id"
    }
  }

  mock_outputs_allowed_terraform_commands = ["validate"]
}
