include "common" {
  path   = find_in_parent_folders("common.hcl")
  expose = true
}

inputs = {
  dot_slash      = fileset("./", "./resources/*.json")
  canonical      = fileset("./", "resources/*.json")
  double_star    = fileset(".", "./resources/**/*.json")
  absolute_path  = fileset(get_terragrunt_dir(), "./resources/*.json")
  parent         = fileset("./resources", "../resources/*.json")
  above          = fileset(".", "../shared/*.json")
  double_slash   = fileset(".", "resources//*.json")
  dot_segment    = fileset(".", "resources/./*.json")
  parent_segment = fileset(".", "resources/sub/../*.json")
  included       = include.common.locals.from_include
}
