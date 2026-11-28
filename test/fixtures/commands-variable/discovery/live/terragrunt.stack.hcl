locals {
  name = commands.accept_parallelism[0]
}

unit "app" {
  source = "../../stack/units/app"
  path   = "${local.name}-${values.suffix}"
}
