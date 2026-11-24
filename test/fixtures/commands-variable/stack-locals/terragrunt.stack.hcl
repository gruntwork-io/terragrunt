locals {
  vars = commands.accept_vars
}

unit "app" {
  source = "../stack/units/app"
  path   = "app"

  values = {
    vars = local.vars
  }
}
