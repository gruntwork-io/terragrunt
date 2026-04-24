locals {
  config_json_files = sort(fileset(get_terragrunt_dir(), "*.json"))
  merged_config = deep_merge([
    for file in local.config_json_files :
    jsondecode(file("${get_terragrunt_dir()}/${file}"))
  ]...)
}

inputs = local.merged_config
