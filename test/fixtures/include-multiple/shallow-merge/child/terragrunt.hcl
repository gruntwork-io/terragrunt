terraform {
  source = "${get_terragrunt_dir()}/../../modules/reflect"
}

include "inputs" {
  path = find_in_parent_folders("terragrunt_inputs.hcl")
}

include "inputs_final" {
  path = find_in_parent_folders("terragrunt_inputs_final.hcl")
}

include "vpc_dep" {
  path = find_in_parent_folders("terragrunt_vpc_dep.hcl")
}

dependency "vpc" {
  config_path = "../vpc"
  mock_outputs = {
    attribute     = "mock"
    old_attribute = "old val"
    new_attribute = "new val"
    list_attr     = ["hello", "mock", "foo"]
    map_attr = {
      foo = "bar"
      bar = "baz"
    }
  }
}

inputs = {
  dep_out = dependency.vpc.outputs
}
