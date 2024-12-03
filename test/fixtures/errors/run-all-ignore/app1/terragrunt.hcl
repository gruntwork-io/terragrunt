errors {
  ignore "example1" {
    ignorable_errors = [
      ".*example1.*",
    ]
    message = "Ignoring error example1"
  }

  ignore "example2" {
    ignorable_errors = [
      ".*example2.*",
    ]
    message = "Ignoring error example2"
  }
}