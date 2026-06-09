# An autoinclude patches a unit with a block other than dependency/generate. Here a
# feature block is injected; it must land in the generated unit and merge into the
# unit's effective config.
unit "svc" {
  source = "../catalog/units/svc"
  path   = "svc"

  autoinclude {
    feature "foo" {
      default = true
    }
  }
}
