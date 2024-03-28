include "root" {
  path = find_in_parent_folders("terragrunt.hcl")
}

include "local" {
  path = "local.hcl"
}

dependency "cluster" {
  config_path = "../cluster"
}

inputs = {
  foo        = "dependency-input-foo-value"
  cluster-id = dependency.cluster.outputs.id
}
