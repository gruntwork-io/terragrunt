terragrunt = {
  terraform {
    # This hook configures Terragrunt to create an empty file called before.out
    # before execution of terragrunt
    before_hook "before_hook_1" {
      commands = ["apply", "plan"]
      execute = ["touch","before.out"]
      run_on_error = true
    }

    # This hook configures Terragrunt to create an empty file called before-child.out
    # This will merge up and override the parent before_hook
    # before execution of terragrunt
    before_hook "before_hook_merge_1" {
      commands = ["apply", "plan"]
      execute = ["touch","before-child.out"]
      run_on_error = true
    }

    # This hook configures Terragrunt to create an empty file called after.out
    # after execution of terragrunt
    after_hook "after_hook_1" {
      commands = ["apply", "plan"]
      execute = ["touch","after.out"]
      run_on_error = true
    }
  }

  include {
    path = "${find_in_parent_folders()}"
  }

}