terraform {
  # This hook echos out user's HOME path or HelloWorld
  before_hook "before_hook_1" {
    commands = ["apply", "plan"]
    execute = ["echo", get_env("HOME", "HelloWorld")]
    run_on_error = true
  }
}