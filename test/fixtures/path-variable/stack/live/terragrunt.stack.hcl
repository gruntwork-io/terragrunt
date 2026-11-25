locals {
  stack_dir = path.stack
}

unit "app" {
  source = "../units/app"
  path   = "app"

  values = {
    stack_dir       = local.stack_dir
    unit_in_stack   = try(path.unit, "no-unit")
    module_in_stack = try(path.download, "no-module")
  }
}
