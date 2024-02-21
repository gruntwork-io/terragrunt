include {
  path = find_in_parent_folders("root-terragrunt.hcl")
}

dependency "dependency_1" {
  config_path = "../dep1"
  mock_outputs = {
    name = "dummy"
  }
}

dependency "dependency_2" {
  config_path = "../dep2"
  mock_outputs = {
    name = "dummy"
  }
}

inputs = {
  name = format(
    "%s:%s",
    dependency.dependency_1.outputs.name,
    dependency.dependency_2.outputs.name,
  )
}