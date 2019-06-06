terraform {
  extra_arguments "test" {
    commands = ["apply"]
    env_vars = {
      TF_VAR_custom_var = "I'm set in extra_arguments env_vars"
    }
  }

  extra_arguments "shouldnotapply" {
    commands = [ "refresh" ]

    env_vars = {
      TF_VAR_custom_var = "I'm only set for refresh command, so will be ignored for apply"
    }

  }
}
