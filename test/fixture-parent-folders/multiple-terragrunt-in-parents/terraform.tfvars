terragrunt = {
  # Configure Terragrunt to automatically store tfstate files in an S3 bucket
  remote_state {
    backend = "s3"
    config {
      encrypt = true
      bucket = "my-bucket"
      key = "${path_relative_to_include()}/terraform.tfstate"
      region = "none"
    }
  }

  terraform {
    extra_arguments "root" {
      commands = ["${get_terraform_commands_that_need_vars()}"]
      env_vars = {
        TF_VAR_root_from = "${path_relative_from_include()}"
        TF_VAR_root_to = "${path_relative_to_include()}"
      }
    }
  }
}
