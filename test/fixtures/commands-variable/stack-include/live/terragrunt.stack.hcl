include "catalog" {
  path = "../catalog/components.hcl"
}

unit "root" {
  source = "../catalog/units/app"
  path   = "root"

  values = {
    list = commands.accept_locking
  }
}
