terragrunt = {
  terraform {
    // source = "test"

    # This hook configures Terragrunt to do a simple echo statement after executing any Terraform command
    after_hook "after_hook_1" {
      commands = ["apply", "plan"]
      execute = ["touch","file.out"]
      run_on_error = true
    }
  }
}