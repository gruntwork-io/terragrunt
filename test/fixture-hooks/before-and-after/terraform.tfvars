terragrunt = {
  terraform {
    // source = "test"

    # This hook configures Terragrunt to copy /foo to /bar before executing apply or plan
    before_hook "before_hook_1" {
      commands = ["apply", "plan"]
      execute = ["touch","before.out"]
      run_on_error = true
    }

    # This hook configures Terragrunt to do a simple echo statement after executing any Terraform command
    after_hook "after_hook_1" {
      commands = ["apply", "plan"]
      execute = ["touch","after.out"]
      run_on_error = true
    }

  }
}