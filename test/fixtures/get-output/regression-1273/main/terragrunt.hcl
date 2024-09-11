dependency "dep" {
  config_path = "../dep"
}

inputs = {
  input = dependency.dep.outputs.output
}
