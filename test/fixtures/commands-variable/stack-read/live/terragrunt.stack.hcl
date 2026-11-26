locals {
  read = read_terragrunt_config("read.hcl")
}

unit "app" {
  source = "../units/app"
  path   = "app"

  values = {
    read_vars  = local.read.locals.vars
    read_input = local.read.inputs.input
  }
}
