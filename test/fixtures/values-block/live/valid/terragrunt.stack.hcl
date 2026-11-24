unit "app" {
  source = "../../catalog/units/app"
  path   = "app"

  values = {
    name           = "web"
    instance_count = "3"
  }
}
