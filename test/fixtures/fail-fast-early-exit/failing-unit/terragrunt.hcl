terraform {
  after_hook "signal" {
    commands     = ["apply"]
    execute      = ["touch", "${get_terragrunt_dir()}/../.fail-signal"]
    run_on_error = true
  }
}
