terraform {
  # This hook is purposely misconfigured to trigger an error
  before_hook "before_hook_1" {
    commands = ["apply", "plan"]
    execute = [""]
    run_on_error = true
  }
}
