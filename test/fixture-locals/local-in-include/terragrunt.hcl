locals {
  parent_terragrunt_dir = get_parent_terragrunt_dir()
  terragrunt_dir = get_terragrunt_dir()
  terraform_command = get_terraform_command()
}

inputs = {
  parent_terragrunt_dir = local.parent_terragrunt_dir
  terragrunt_dir = local.terragrunt_dir
  terraform_command = local.terraform_command
}
