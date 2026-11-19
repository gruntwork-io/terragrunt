inputs = {
  auto_unit   = path.unit
  auto_module = path.module
  auto_stack  = try(path.stack, "no-stack")
}
