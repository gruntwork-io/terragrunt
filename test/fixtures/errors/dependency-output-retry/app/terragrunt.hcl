dependency "dep" {
  config_path = "../dependency"
}

inputs = {
  result = dependency.dep.outputs.result
}
