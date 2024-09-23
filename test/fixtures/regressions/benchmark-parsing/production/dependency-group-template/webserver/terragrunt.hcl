include {
  path = find_in_parent_folders("root-terragrunt.hcl")
}

locals {
  environment_vars = read_terragrunt_config(find_in_parent_folders("environment.hcl")).locals
  app_vars         = read_terragrunt_config(find_in_parent_folders("app.hcl")).locals
}

terraform {
  source = "${get_terragrunt_dir()}/modules/dummy-module"
}

inputs = {
  name = "${local.environment_vars.environment}-${local.app_vars.name}"
}
