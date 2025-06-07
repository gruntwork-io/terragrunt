terraform {
  source = "."

  error_hook "create_file" {
    commands  = ["apply"]
    execute   = ["touch", "success.txt"]
    on_errors = [".*"]
  }
}

errors {
  retry "file_not_there_yet" {
    max_attempts       = 2
    sleep_interval_sec = 1
    retryable_errors   = [".*"]
  }
}
