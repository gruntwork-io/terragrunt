include {
  path = find_in_parent_folders("root-terragrunt.hcl")
}

locals {
  environment_vars = read_terragrunt_config(find_in_parent_folders("environment.hcl")).locals
  app_vars         = read_terragrunt_config(find_in_parent_folders("app.hcl")).locals
}

# Create dependencies to require PartialParseConfig to be fired multiple times
# with a similar file content so that the caching is used.
# The more similar dependencies, the more potential cache reuse.
# If more dependency lookups are desired for benchmarking/testing,
# create new symlinks from dependency-group-template/
# and add the dependencies below.
dependency "dependency_group_1" {
  config_path = "../../dependency-group-1/webserver"
  mock_outputs = {
    name = "mock-name"
  }
}

dependency "dependency_group_2" {
  config_path = "../../dependency-group-2/webserver"
  mock_outputs = {
    name = "mock-name"
  }
}

dependency "dependency_group_3" {
  config_path = "../../dependency-group-3/webserver"
  mock_outputs = {
    name = "mock-name"
  }
}

dependency "dependency_group_4" {
  config_path = "../../dependency-group-4/webserver"
  mock_outputs = {
    name = "mock-name"
  }
}

terraform {
  source = "${get_terragrunt_dir()}/modules/dummy-module"
}

inputs = {
  name = "${local.environment_vars.environment}-${local.app_vars.name}"

  dependency_output = [
    dependency.dependency_group_1.outputs.name,
    dependency.dependency_group_2.outputs.name,
    dependency.dependency_group_3.outputs.name,
    dependency.dependency_group_4.outputs.name,
  ]
}
