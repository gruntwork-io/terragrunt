include "root" {
  path           = find_in_parent_folders("root.hcl")
  expose         = true
  merge_strategy = "no_merge"
}

exclude {
  if      = include.root.locals.skip
  actions = ["plan"]
}
