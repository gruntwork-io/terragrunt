terraform {
  source = "git::ssh://git@github.com/gruntwork-io/another-dont-exist.git//fixtures/source-map/modules/app?ref=master"
}

dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  name   = "terragrunt"
  vpc_id = dependency.vpc.outputs.vpc_id
}
