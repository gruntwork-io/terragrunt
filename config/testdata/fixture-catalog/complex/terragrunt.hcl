locals {
  common_vars = read_terragrunt_config(find_in_parent_folders("common.hcl"))
}

catalog {
  urls = [
    "https://github.com/gruntwork-io/terraform-aws-utilities",
  ]
}

inputs = {
  name_prefix = local.common_vars.locals.name_prefix
}
