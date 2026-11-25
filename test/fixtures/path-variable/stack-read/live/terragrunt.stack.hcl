locals {
  read      = read_terragrunt_config("read.hcl")
  dependent = read_terragrunt_config("../dependent/terragrunt.hcl")
}

unit "app" {
  source = "../units/app"
  path   = "app"

  values = {
    read_stack      = local.read.locals.stack
    read_unit       = local.read.locals.unit
    read_download   = local.read.locals.download
    nested_stack    = local.read.locals.nested_stack
    dependent_stack = local.dependent.inputs.stack
    dependent_base  = local.dependent.inputs.base_id
  }

  autoinclude {
    inputs = {
      auto_read_stack = local.read.locals.stack
      auto_read_unit  = local.read.locals.unit
    }
  }
}
