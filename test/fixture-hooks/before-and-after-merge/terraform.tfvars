terragrunt = {
  # Configure Terragrunt to automatically store tfstate files in an S3 bucket
  remote_state {
    backend = "s3"
    config {
      encrypt = true
      bucket = "__FILL_IN_BUCKET_NAME__"
      key = "${path_relative_to_include()}/terraform.tfstate"
      region = "us-west-2"
    }
  }

  terraform {
    # This hook configures Terragrunt to attempt to create an empty file called before-parent.out
    # This will be overridden by the child before_hook
    # before execution of terragrunt
    before_hook "before_hook_merge_1" {
      commands = ["apply", "plan"]
      execute = ["touch","before-parent.out"]
      run_on_error = true
    }

    # This hook configures Terragrunt to create an empty file called after-parent.out
    # after execution of terragrunt
    after_hook "after_hook_parent_1" {
      commands = ["apply", "plan"]
      execute = ["touch","after-parent.out"]
      run_on_error = true
    }
  }
}