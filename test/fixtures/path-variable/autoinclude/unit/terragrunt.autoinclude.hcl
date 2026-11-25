inputs = {
  auto_unit   = path.unit
  auto_module = path.download
  auto_stack  = try(path.stack, "no-stack")
}
