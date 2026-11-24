include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

inputs = {
  unit            = path.unit
  exposed_include = include.root.locals.included_file
}
