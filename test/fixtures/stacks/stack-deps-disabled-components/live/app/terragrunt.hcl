terraform {
  source = "."
}

dependency "networking" {
  config_path = "../networking"
}

inputs = {
  vpc_id = dependency.networking.outputs.vpc.vpc_id
}
