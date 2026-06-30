locals {
  common = read_terragrunt_config("${get_terragrunt_dir()}/common.hcl")
}

inputs = {
  dep_name = local.common.inputs.dep_name
}
