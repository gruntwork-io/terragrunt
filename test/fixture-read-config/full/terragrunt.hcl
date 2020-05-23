locals {
  config = read_terragrunt_config("${get_terragrunt_dir()}/source.hcl")
}

inputs = {
  localstg                     = local.config.locals
  generate                     = local.config.generate
  remote_state                 = local.config.remote_state
  terraformtg                  = local.config.terraform
  dependencies                 = local.config.dependencies
  terraform_binary             = local.config.terraform_binary
  terraform_version_constraint = local.config.terraform_version_constraint
  terragrunt_version_constraint = local.config.terragrunt_version_constraint
  download_dir                 = local.config.download_dir
  prevent_destroy              = local.config.prevent_destroy
  skip                         = local.config.skip
  iam_role                     = local.config.iam_role
  inputs                       = local.config.inputs
}
