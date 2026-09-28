include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "."
}

# tomap() keeps the mocks a map rather than an object literal, which is the form a stack dependency
# has to accept as well: subnets has no state, so its mock only resolves if maps are honored.
dependency "networking" {
  config_path = "../networking"

  mock_outputs = tomap({
    vpc = {
      id = "mock-vpc-id"
    }
    subnets = {
      id = "mock-subnet-id"
    }
  })
}

inputs = {
  vpc_id    = dependency.networking.outputs.vpc.id
  subnet_id = dependency.networking.outputs.subnets.id
}
