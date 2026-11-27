dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    ghost = "ghost-mock"
  }
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
  nope   = dependency.vpc.outputs.nope
}
