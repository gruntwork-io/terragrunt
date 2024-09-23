terraform {
  source = "${get_terragrunt_dir()}/../../modules/reflect"
}

include "inputs" {
  path           = find_in_parent_folders("terragrunt_inputs.hcl")
  merge_strategy = "deep"
}

include "inputs_override" {
  path = find_in_parent_folders("terragrunt_inputs_override.hcl")
}

# NOTE: This shallow merge is expected to be a noop, as the deep merge between vpc_dep and the child config completes
# the expected dependency.vpc block.
include "vpc_dep_override" {
  path = find_in_parent_folders("terragrunt_vpc_dep_override.hcl")
}

include "vpc_dep" {
  path           = find_in_parent_folders("terragrunt_vpc_dep.hcl")
  merge_strategy = "deep"
}


dependency "vpc" {
  config_path = "../vpc"
  mock_outputs = {
    attribute     = "mock"
    new_attribute = "new val"
    list_attr     = ["mock", "foo"]
    map_attr = {
      bar = "baz"
    }
  }
}

inputs = {
  attribute = "mock"
  list_attr = ["mock", "foo"]

  dep_out = dependency.vpc.outputs
}
