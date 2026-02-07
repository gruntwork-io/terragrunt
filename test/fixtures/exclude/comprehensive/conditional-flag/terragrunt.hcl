include "flags" {
  path = find_in_parent_folders("flags.hcl")
}

terraform {
  source = "."
}

# Excluded based on feature flag
exclude {
  if      = feature.exclude.value
  actions = ["all"]
}
