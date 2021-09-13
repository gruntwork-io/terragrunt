terraform {
  source = "${get_terragrunt_dir()}/../../modules/reflect"
}

include "inputs" {
  path           = find_in_parent_folders("terragrunt_inputs.hcl")
  merge_strategy = "deep"
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
  attribute     = "mock"
  new_attribute = "new val"
  list_attr     = ["mock", "foo"]
  map_attr = {
    bar = "baz"
  }

  dep_out = dependency.vpc.outputs
}
