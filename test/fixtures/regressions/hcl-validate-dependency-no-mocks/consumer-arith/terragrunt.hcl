dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  n = dependency.vpc.outputs.num + 1
}
