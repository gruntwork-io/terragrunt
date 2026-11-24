stack "net" {
  source = "../stacks/net"
  path   = "net"
}

unit "app" {
  source = "../units/app"
  path   = "app"

  values = {
    stack_dir = path.stack
    net_dir   = stack.net.path
  }
}
