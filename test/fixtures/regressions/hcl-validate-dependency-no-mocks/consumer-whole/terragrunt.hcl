dependency "vpc" {
  config_path = "../vpc"
}

inputs = dependency.vpc.outputs
