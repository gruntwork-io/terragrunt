terraform {
  extra_arguments "varfiles" {
    commands           = get_terraform_commands_that_need_vars()
    required_var_files = ["${get_terragrunt_dir()}/varfiles/main.tfvars"]
  }
}
