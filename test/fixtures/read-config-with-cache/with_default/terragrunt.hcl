locals {
  config_does_not_exist = read_terragrunt_config("--terragrunt-with-cache", "${get_terragrunt_dir()}/i-dont-exist.hcl", {data = "default value"})
}

inputs = local.config_does_not_exist
