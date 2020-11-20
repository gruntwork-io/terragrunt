terraform {
  # This hook configures Terragrunt to create an empty file called before.out
  # before execution of terragrunt
  before_hook "before_hook_1" {
    commands = ["apply", "plan"]
    execute = ["touch","before.out"]
    run_on_error = true
  }

  # This hook configures Terragrunt to create an empty file called after.out
  # after execution of terragrunt
  after_hook "after_hook_1" {
    commands = ["apply", "plan"]
    execute = ["touch","after.out"]
    run_on_error = true
  }

  before_hook "before_hook_2" {
    commands = ["terragrunt-read-config"]
    execute = ["echo", "BEFORE_TERRAGRUNT_READ_CONFIG"]
    run_on_error = true
  }

  after_hook "after_hook_2" {
    commands = ["terragrunt-read-config"]
    execute = ["echo", "AFTER_TERRAGRUNT_READ_CONFIG"]
    run_on_error = true
  }
}
