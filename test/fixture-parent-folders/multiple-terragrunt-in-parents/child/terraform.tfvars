terragrunt = {
  include {
    path = "${find_in_parent_folders()}"
  }

  remote_state {
    config {
      region = "us-east-1"
    }
  }

  terraform {
    extra_arguments "child" {
      commands = ["${get_terraform_commands_that_need_vars()}"]
      env_vars = {
        TF_VAR_child_from = "${path_relative_from_include()}"
        TF_VAR_child_to = "${path_relative_to_include()}"
      }
    }
  }
}

