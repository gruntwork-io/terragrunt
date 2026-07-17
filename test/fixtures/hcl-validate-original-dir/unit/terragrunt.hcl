# get_original_terragrunt_dir() must resolve to this unit, not the parent launch dir
locals {
  common = read_terragrunt_config("${get_original_terragrunt_dir()}/../common.hcl")
}

inputs = {
  input = local.common.locals.example
}
