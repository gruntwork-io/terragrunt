locals {
  stack_dir = stack.path
}

unit "app" {
  source = "../units/app"
  path   = "app"

  values = {
    stack_dir       = local.stack_dir
    unit_in_stack   = try(unit.path, "no-unit")
    module_in_stack = try(path.download, "no-module")
  }
}
