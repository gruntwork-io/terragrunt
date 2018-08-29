terragrunt = {
  terraform = {
    extra_arguments "test" {
      env_vars = {
        TF_VAR_custom_var = "I'm set in extra_arguments env_vars"
      }
    }
  }
}
