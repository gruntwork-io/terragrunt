unit "app" {
  source = "../units/app"
  path   = "app"

  autoinclude {
    inputs = {
      auto_vars = commands.accept_vars
    }
  }
}
