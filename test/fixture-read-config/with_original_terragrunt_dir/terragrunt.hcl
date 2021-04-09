locals {
  bar = read_terragrunt_config("foo/bar.hcl")
}

inputs = {
  terragrunt_dir          = local.bar.locals.terragrunt_dir
  original_terragrunt_dir = local.bar.locals.original_terragrunt_dir
}