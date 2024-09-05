include {
  path = find_in_parent_folders("root-terragrunt.hcl")
}

include "environment_vars" {
  path   = find_in_parent_folders("environment.hcl")
  expose = true
}

include "app_vars" {
  path   = find_in_parent_folders("app.hcl")
  expose = true
}

terraform {
  source = "${get_terragrunt_dir()}/modules/dummy-module"
}

inputs = {
  name = "${include.environment_vars.locals.environment}-${include.app_vars.locals.name}"
}
