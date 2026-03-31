include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

dependency "dep" {
  config_path = "../dependency"
}

terraform {
  source = "."
}

locals {
  common = include.root.locals.common
}

inputs = {
  value = "${local.common}-${dependency.dep.outputs.value}"
}
