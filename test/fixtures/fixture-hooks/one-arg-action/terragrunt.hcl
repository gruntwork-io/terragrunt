terraform {
  # This hook tests execution of agrgs that take no parameters
  before_hook "before_hook_1" {
    commands = ["apply", "plan"]
    execute = ["date"]
    run_on_error = true
  }
}
