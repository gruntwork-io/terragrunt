errors {

  retry "script_errors" {
    retryable_errors = [".*Script error.*"]
    max_attempts       = 3
    sleep_interval_sec = 2
  }

  retry "aws_errors" {
    retryable_errors = [".*AWS error.*"]
    max_attempts       = 3
    sleep_interval_sec = 2
  }

}