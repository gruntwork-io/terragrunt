# An autoinclude patches a unit with a block other than dependency/generate. Here an
# errors block with an ignore rule is injected; it must land in the generated unit and
# merge into the unit's effective config.
unit "svc" {
  source = "../catalog/units/svc"
  path   = "svc"

  autoinclude {
    errors {
      ignore "bar" {
        ignorable_errors = [".*bar.*"]
        message          = "Ignoring error bar"

        signals = {
          failed_bar = true
        }
      }
    }
  }
}
