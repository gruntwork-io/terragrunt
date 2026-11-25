include "catalog" {
  path = "../catalog/components.hcl"
}

unit "root" {
  source = "../catalog/units/app"
  path   = "root"

  values = {
    include   = try(path.include, "no-include")
    stack_dir = path.stack
  }
}
