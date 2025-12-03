errors {
  retry "transient_errors" {
    retryable_errors = [".*Transient error.*"]
    max_attempts = 3
    sleep_interval_sec = 1
  }
}

terraform {
  before_hook "simulate_transient_error" {
    commands = ["apply"]
    execute  = ["bash", "-c", "if [ ! -f success.txt ]; then echo 'Transient error - will succeed on retry' >&2 && touch success.txt && exit 1; fi"]
  }
}
