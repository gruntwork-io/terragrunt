locals {
  tfvars = jsondecode(read_tfvars_file("shadowed.tfvars"))
  config = read_terragrunt_config("shadowed.hcl")
}

inputs = {
  tfvars_origin = local.tfvars.origin
  config_origin = local.config.locals.origin
}
