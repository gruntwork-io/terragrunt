dependency "vpc" {
  config_path = "../vpc"
}

variables {
  kind = "tfvar"

  inputs = {
    vpc_id     = dependency.vpc.outputs.vpc_id
    subnet_ids = dependency.vpc.outputs.subnet_ids
  }
}
