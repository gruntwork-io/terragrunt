locals {
  parent_terragrunt_dir = get_parent_terragrunt_dir()
  terragrunt_dir = get_terragrunt_dir()
  terraform_command = get_terraform_command()
  terraform_cli_args = get_terraform_cli_args()
}

inputs = {
  parent_terragrunt_dir = local.parent_terragrunt_dir
  terragrunt_dir = local.terragrunt_dir
  terraform_command = local.terraform_command
  terraform_cli_args = local.terraform_cli_args
}
