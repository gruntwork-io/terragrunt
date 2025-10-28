errors {
  retry "default_errors" {
    retryable_errors = get_default_retryable_errors()
    max_attempts = 5
    sleep_interval_sec = 0
  }
}
