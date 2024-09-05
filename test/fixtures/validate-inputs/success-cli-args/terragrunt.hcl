terraform {
  extra_arguments "varfiles" {
    commands  = get_terraform_commands_that_need_vars()
    arguments = ["-var=other_input=world", "-var-file=${get_terragrunt_dir()}/varfiles/main.tfvars"]
  }
}
