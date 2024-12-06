include "root" {
  path           = find_in_parent_folders("root.hcl")
  expose         = true
}

dependency "dep" {
  config_path = include.root.inputs.dep_path
}

inputs = {
  region = "${include.root.locals.region}-${dependency.dep.outputs.env}"
}
