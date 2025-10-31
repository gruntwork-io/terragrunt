terraform {
  source = "."
}

dependency "vpc" {
  config_path = "../vpc"
  mock_outputs = {
    vpc_id = "vpc-12345"
  }
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
