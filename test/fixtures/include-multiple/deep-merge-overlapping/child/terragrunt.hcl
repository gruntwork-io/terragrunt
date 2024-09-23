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

include "inputs_override" {
  path           = find_in_parent_folders("terragrunt_inputs_override.hcl")
  merge_strategy = "deep"
}

include "vpc_dep_override" {
  path           = find_in_parent_folders("terragrunt_vpc_dep_override.hcl")
  merge_strategy = "deep"
}

dependency "vpc" {
  config_path = "../vpc"
  mock_outputs = {
    attribute = "mock"
    list_attr = ["foo"]
  }
}

inputs = {
  attribute = "mock"
  list_attr = ["foo"]

  dep_out = dependency.vpc.outputs
}
