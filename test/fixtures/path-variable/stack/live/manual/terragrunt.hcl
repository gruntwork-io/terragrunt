inputs = {
  unit         = path.unit
  stack        = try(path.stack, "no-stack")
  parent_stack = path.parentstack
}
