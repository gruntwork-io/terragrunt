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

  retry "script_errors" {
    retryable_errors = [".*Script error.*"]
    max_attempts = 3
    sleep_interval_sec = 2
  }

  retry "aws_errors" {
    retryable_errors = [".*AWS error.*"]
    max_attempts = 3
    sleep_interval_sec = 2
  }

}