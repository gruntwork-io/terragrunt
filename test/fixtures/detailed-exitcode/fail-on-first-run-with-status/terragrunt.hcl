errors {
  retry "all_errors" {
    retryable_errors = [".*"]
    max_attempts = 2
    sleep_interval_sec = 1
  }
}
