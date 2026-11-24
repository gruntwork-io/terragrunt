inputs = {
  auto_unit   = unit.path
  auto_module = path.download
  auto_stack  = try(stack.path, "no-stack")
}
