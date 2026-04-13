locals {
  original_dir    = get_original_terragrunt_dir()
  common_config   = read_terragrunt_config("${local.original_dir}/_common.hcl")
  app_name        = local.common_config.locals.app_name
}
