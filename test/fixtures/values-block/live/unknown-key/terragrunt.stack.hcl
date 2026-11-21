unit "app" {
  source = "../../catalog/units/app"
  path   = "app"

  values = {
    name = "web"
    tag  = { team = "core" }
  }
}
