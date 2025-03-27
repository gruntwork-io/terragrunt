locals {
  config = read_terragrunt_config("terragrunt.values.hcl")
}

inputs = {
  project = local.config.project
  env = local.config.env
  data = local.config.data
}