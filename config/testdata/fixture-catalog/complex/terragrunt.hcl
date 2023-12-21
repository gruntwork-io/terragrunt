locals {
  common_vars = read_terragrunt_config(find_in_parent_folders("common.hcl"))
}

catalog {
  urls = [
    "https://github.com/${local.common_vars.locals.github_org}/terraform-aws-utilities",
  ]
}

inputs = {
  github_org = local.common_vars.locals.github_org
}
