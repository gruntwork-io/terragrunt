dependency "stack" {
  config_path = "../live"
}

inputs = {
  vpc_id = dependency.stack.outputs.vpc.vpc_id
}
