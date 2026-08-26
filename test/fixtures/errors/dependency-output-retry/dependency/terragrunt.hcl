errors {
  retry "transient_dependency_output" {
    retryable_errors   = [".*Transient dependency output error.*"]
    max_attempts       = 2
    sleep_interval_sec = 1
  }
}

terraform {
  before_hook "simulate_transient_output_error" {
    commands = ["output"]
    execute  = ["bash", "-c", "if [ ! -f output-ready.txt ]; then echo 'Transient dependency output error' >&2 && touch output-ready.txt && exit 1; fi"]
  }
}
