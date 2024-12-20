include {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

locals {
  environment   = "test"
  parent_region = "${include.locals.region}-${local.environment}"
}

inputs = {
  region = local.parent_region
}
