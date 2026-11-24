unit "app" {
  source = "../stack/units/app"
  path   = "app"

  values = {
    stack_dir       = values.stack_dir
    unit_in_stack   = values.unit_in_stack
    module_in_stack = try(path.download, "no-module")
    parent_in_stack = try(path.parentstack, "no-parent")
  }
}
