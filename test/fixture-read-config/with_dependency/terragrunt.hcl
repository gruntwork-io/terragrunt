locals {
  config_with_dependency = read_terragrunt_config("${get_terragrunt_dir()}/dep/terragrunt.hcl")
}

terraform {
  source = "${get_terragrunt_dir()}/../../fixture-inputs"
}

inputs = local.config_with_dependency.dependency.inputs.outputs
