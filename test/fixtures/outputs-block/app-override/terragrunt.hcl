dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    vpc_id = "vpc-override"
  }
}

inputs = {
  vpc_id     = dependency.vpc.outputs.vpc_id
  subnet_ids = dependency.vpc.outputs.subnet_ids
}
