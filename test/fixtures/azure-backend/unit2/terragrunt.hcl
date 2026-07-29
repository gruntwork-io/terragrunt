terraform {
  source = "tfr://registry.terraform.io/yorinasub17/terragrunt-registry-test/null//modules/one?version=0.0.2"
}

include "common" {
  path = find_in_parent_folders("common.hcl")
}
