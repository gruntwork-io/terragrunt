terragrunt = {
  terraform {

    before_hook "before_hook_1" {
      commands = ["apply", "plan"]
      execute = ["date"]
      run_on_error = true
    }
  }
}