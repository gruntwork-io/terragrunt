include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

terraform {
  source = include.root.locals.module_source
}

dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    value = "hello"
  }
}
