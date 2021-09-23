dependency "dep" {
  config_path = "../dep"
}

inputs = {
  env  = "qa"
  name = dependency.dep.outputs.name
}
