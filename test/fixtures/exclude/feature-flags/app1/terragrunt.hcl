include "flags" {
  path   = find_in_parent_folders("flags.hcl")
}

exclude {
  if = feature.exclude1.value
  actions = ["all"]
  exclude_dependencies = true
}
