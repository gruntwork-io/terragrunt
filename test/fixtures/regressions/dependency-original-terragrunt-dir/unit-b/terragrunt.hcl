locals {
  common   = read_terragrunt_config("common.hcl")
  app_name = local.common.locals.app_name
}

inputs = {
  app_name = local.app_name
}
