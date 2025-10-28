errors {
  retry "custom_errors" {
    retryable_errors = [
      "My own little error"
    ]
    max_attempts = 3
    sleep_interval_sec = 5
  }
}
