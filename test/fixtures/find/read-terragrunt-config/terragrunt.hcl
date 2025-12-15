locals {
  common_deps = read_terragrunt_config("${get_terragrunt_dir()}/common_deps.hcl")
}

terraform {
  source = "."
}

inputs = {
  value = local.common_deps.dependency.module.outputs.value
  module_value = local.common_deps.inputs.module_value
}

