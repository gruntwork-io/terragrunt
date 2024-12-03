errors {
  ignore "example1" {
    ignorable_errors = [
      ".*example1.*",
    ]
    message = "Ignoring error example1"

    signals = {
      failed          = false
      failed_example1 = true
      message         = "Failed example1"
    }
  }

  ignore "example2" {
    ignorable_errors = [
      ".*example2.*",
    ]
    message = "Ignoring error example2"

    signals = {
      failed          = false
      failed_example2 = true
      message         = "Failed example2"
    }
  }
}