include "root" {
  path = find_in_parent_folders("root.hcl")
  expose = true
}

terraform {
  source = "${get_terragrunt_dir()}/."
}

locals {
    name = "service1"
}

inputs = {
    name = local.name
}
