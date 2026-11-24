locals {
  stack_dir = stack.path
}

stack "path" {
  source = "../stacks/net"
  path   = "named"
}

unit "app" {
  source = "../units/app"
  path   = "app"

  values = {
    named_dir = stack.path.path
    stack_dir = local.stack_dir
  }
}
