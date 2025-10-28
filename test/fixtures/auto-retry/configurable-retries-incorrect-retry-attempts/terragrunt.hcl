errors {
  retry "test_errors" {
    retryable_errors = get_default_retryable_errors()
    max_attempts = 0
    sleep_interval_sec = 5
  }
}
