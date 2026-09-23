dependency "dep" {
  config_path = "../dep"
}

inputs = {
  x = dependency.dep.outputs.x
}
