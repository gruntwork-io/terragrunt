dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
  nat_id = dependency.vpc.outputs.nat_id
}
