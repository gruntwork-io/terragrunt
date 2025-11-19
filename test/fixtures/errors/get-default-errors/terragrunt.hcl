errors {
  retry "default_errors" {
    retryable_errors = get_default_retryable_errors()
    max_attempts = 3
    sleep_interval_sec = 5
  }

  retry "custom_errors" {
    retryable_errors = ["my special snowflake"]
    max_attempts = 2
    sleep_interval_sec = 1
  }
}

inputs = {
  default_retryable_errors = get_default_retryable_errors()
  custom_error = "my special snowflake"
}
