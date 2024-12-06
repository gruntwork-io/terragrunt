include {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = "deep"
}

dependency "vpc" {
  config_path = "../vpc"
  mock_outputs = {
    attribute     = "mock"
    new_attribute = "new val"
    list_attr     = ["mock"]
    map_attr = {
      bar = "baz"
    }
  }
}

inputs = {
  attribute     = "mock"
  new_attribute = "new val"
  list_attr     = ["mock"]
  map_attr = {
    bar = "baz"
  }

  dep_out = dependency.vpc.outputs
}
