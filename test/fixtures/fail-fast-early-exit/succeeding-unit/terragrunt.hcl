terraform {
  before_hook "wait_for_signal" {
    commands = ["apply"]
    execute  = [
      "bash", "-c",
      "for i in $(seq 1 30); do [ -f '${get_terragrunt_dir()}/../.fail-signal' ] && exit 0; sleep 0.1; done; exit 0"
    ]
  }
}
