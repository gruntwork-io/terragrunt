errors {
  retry "transient_fail" {
    retryable_errors = ["(?s).*transient fail.*"]
    max_attempts       = 2
    sleep_interval_sec = 1
  }
}