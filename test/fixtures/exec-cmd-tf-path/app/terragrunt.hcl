terraform {
  source = "."
}

dependency "dep" {
    config_path = "../dep"
}

inputs = {
  baz = dependency.dep.outputs.baz
}
