# An autoinclude patches a unit with a block other than dependency/generate. Here an
# errors block with a retry rule is injected; it must land in the generated unit and
# merge into the unit's effective config.
unit "svc" {
  source = "../catalog/units/svc"
  path   = "svc"

  autoinclude {
    errors {
      retry "transient" {
        max_attempts       = 3
        sleep_interval_sec = 5
        retryable_errors   = [".*transient.*"]
      }
    }
  }
}
