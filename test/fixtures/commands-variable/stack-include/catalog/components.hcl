unit "app" {
  source = "../catalog/units/app"
  path   = "catalog-${commands.accept_parallelism[0]}"

  values = {
    list = commands.accept_input
  }
}
