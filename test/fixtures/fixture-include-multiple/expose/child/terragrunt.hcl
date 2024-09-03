terraform {
  source = "${get_terragrunt_dir()}/../../modules/reflect"
}

include "inputs_override" {
  path           = find_in_parent_folders("terragrunt_inputs_override.hcl")
  expose         = true
  merge_strategy = "no_merge"
}

include "vpc_dep" {
  path           = find_in_parent_folders("terragrunt_vpc_dep_for_expose.hcl")
  expose         = true
  merge_strategy = "no_merge"
}

dependency "vpc" {
  config_path = include.vpc_dep.dependency.vpc.config_path
  mock_outputs = merge(
    include.vpc_dep.dependency.vpc.mock_outputs,
    {
      attribute     = "mock"
      new_attribute = "new val"
      list_attr     = ["hello", "mock", "foo"]
      map_attr = {
        foo = "bar"
        bar = "baz"
      }
    },
  )
  mock_outputs_allowed_terraform_commands = include.vpc_dep.dependency.vpc.mock_outputs_allowed_terraform_commands
}

inputs = merge(
  include.inputs_override.inputs,
  {
    attribute     = "mock"
    old_attribute = "old val"
    list_attr     = ["hello", "mock", "foo"]
    map_attr = {
      bar  = "baz"
      foo  = "bar"
      test = dependency.vpc.outputs.new_attribute
    }
    dep_out = dependency.vpc.outputs
  },
)
