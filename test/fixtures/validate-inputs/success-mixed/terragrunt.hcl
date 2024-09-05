terraform {
  extra_arguments "variables" {
    commands           = get_terraform_commands_that_need_vars()
    required_var_files = ["${get_terragrunt_dir()}/varfiles/main.tfvars"]
    env_vars = {
      TF_VAR_input_from_env_var = "from env var"
    }
  }
}

inputs = {
  input_from_input = "from input"
}
