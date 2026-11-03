locals {
  common = read_terragrunt_config("../common.hcl")
}

inputs = {
  secret = local.common.locals.secret
}
