
errors {

  retry "script_errors" {
    retryable_errors = [".*Script error.*"]
    max_attempts = 3
    sleep_interval_sec = 2
  }

}