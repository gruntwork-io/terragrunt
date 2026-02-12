include "flags" {
  path = find_in_parent_folders("flags.hcl")
}

terraform {
  source = "."
}

dependency "dep" {
  config_path = "../dep-unit"

  mock_outputs = {
    data = "mock"
  }
}

# Excluded based on feature flag, with dependency exclusion
exclude {
  if                   = feature.exclude.value
  actions              = ["all"]
  exclude_dependencies = feature.exclude_deps.value
}
