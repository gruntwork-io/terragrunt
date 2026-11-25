dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
  port   = dependency.vpc.outputs.port
  secret = try(dependency.vpc.outputs.secret, "no-mock-for-secret")
}
