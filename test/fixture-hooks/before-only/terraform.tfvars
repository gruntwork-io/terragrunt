terragrunt = {
  terraform {

    before_hook "before_hook_1" {
      commands = ["apply", "plan"]
      execute = ["touch","file.out"]
      run_on_error = true
    }
  }
}