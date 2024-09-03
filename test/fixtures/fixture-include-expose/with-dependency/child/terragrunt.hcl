include "root" {
  path           = find_in_parent_folders()
  expose         = true
  merge_strategy = "deep"
}

inputs = {
  region = "${include.root.locals.region}-${dependency.dep.outputs.env}"
}
