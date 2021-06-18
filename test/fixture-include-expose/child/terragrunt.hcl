include {
  path = find_in_parent_folders()
  expose = true
}

locals {
  parent_region = include.locals.region
}

inputs = {
  region = local.parent_region
}
