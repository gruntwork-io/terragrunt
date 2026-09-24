terraform {
  source = "."
}

dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    skip = true
  }
}

# Deprecated: warns by default, errors under the exclude-dependency-outputs strict control
exclude {
  if      = dependency.dep.outputs.skip
  no_run  = true
  actions = ["plan"]
}
