locals {
  # Automatically load catalog variables shared across all accounts
  catalog_vars = read_terragrunt_config(find_in_parent_folders("common.hcl"))

  # Automatically load inputs variables shared across all accounts
  inputs_vars = read_terragrunt_config(find_in_parent_folders("no-exist.hcl"))

  # Automatically load account-level variables
  account_vars = read_terragrunt_config(find_in_parent_folders("account.hcl"))

  # Automatically load region-level variables
  region_vars = read_terragrunt_config("region.hcl")
}


catalog {
  urls = [
    "dev/us-west-1/modules/terraform-aws-eks",
    "./terraform-aws-service-catalog",
    "https://github.com/${local.catalog_vars.locals.github_org}/terraform-aws-utilities",
  ]
}

inputs = {
  github_org = local.inputs_vars.locals.github_org
}
