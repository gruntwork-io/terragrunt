include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

terraform {
  source = "."
}

locals {
  common = include.root.locals.common
}

inputs = {
  value = local.common
}
