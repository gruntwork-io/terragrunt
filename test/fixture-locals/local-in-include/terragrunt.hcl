locals {
  parent_terragrunt_dir = get_parent_terragrunt_dir()
  terragrunt_dir = get_terragrunt_dir()
}

inputs = {
  parent_terragrunt_dir = local.parent_terragrunt_dir
  terragrunt_dir = local.terragrunt_dir
}
