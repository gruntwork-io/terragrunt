locals {
  vars = commands.accept_vars
}

unit "app" {
  source = "../units/app"
  path   = "app"

  values = {
    vars    = local.vars
    locking = commands.accept_locking
    input   = values.input
  }
}
