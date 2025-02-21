locals {
  run_hook = true
}

terraform {
  before_hook "run_this_one" {
    if = local.run_hook
    commands = ["apply", "plan"]
    execute = ["echo", "running before hook"]
  }

  after_hook "skip_this_one" {
    if = !local.run_hook
    commands = ["apply", "plan"]
    execute = ["echo", "skip after hook"]
  }
}
