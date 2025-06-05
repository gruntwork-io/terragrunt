terraform {
  source = "."
}

errors {
  retry "file_not_there_yet" {
    max_attempts       = 2
    sleep_interval_sec = 1
    retryable_errors   = [".*"]
  }
}
