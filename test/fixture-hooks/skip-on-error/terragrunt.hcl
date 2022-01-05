terraform {
  # This hook purposely causes an error
  before_hook "before_hook_1" {
    commands = ["apply", "plan"]
    execute = ["exit","1"]
    run_on_error = true
  }

  before_hook "before_hook_2" {
    commands = ["apply", "plan"]
    execute = ["echo","BEFORE_NODISPLAY"]
    run_on_error = false
  }

  before_hook "before_hook_3" {
    commands = ["apply", "plan"]
    execute = ["echo","BEFORE_SHOULD_DISPLAY"]
    run_on_error = true
  }

  after_hook "after_hook_1" {
    commands = ["apply", "plan"]
    execute = ["echo","AFTER_NODISPLAY"]
    run_on_error = false
  }

  after_hook "after_hook_1" {
    commands = ["apply", "plan"]
    execute = ["echo","AFTER_SHOULD_DISPLAY"]
    run_on_error = true
  }
  error_hook "error_hook" {
    commands  = ["apply", "plan"]
    execute   = ["echo", "ERROR_HOOK_EXECUTED"]
    on_errors = [".*"]
  }
}
