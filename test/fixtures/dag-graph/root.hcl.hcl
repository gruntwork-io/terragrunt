errors {
  retry "common_errors" {
    retryable_errors = [
      "Error acquiring the state lock",
      "Plugin did not respond",
      "Request cancelled",
      "request was cancelled",
      "InvalidIdentityToken"
    ]
    max_attempts = 3
    sleep_interval_sec = 5
  }
}
