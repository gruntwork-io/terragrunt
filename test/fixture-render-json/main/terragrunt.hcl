terraform {
  source = "./module"
}

include "root" {
  path = find_in_parent_folders()
}

include "envcommon" {
  path = find_in_parent_folders("common_vars.hcl")
}

inputs = {
  type = "main"
}
