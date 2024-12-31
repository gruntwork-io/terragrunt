include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

include "env" {
  path   = find_in_parent_folders("terragrunt_env.hcl")
  expose = true
}

locals {
  parent_region = "${include.root.locals.region}-${include.env.locals.environment}"
}

inputs = {
  region = local.parent_region
}
