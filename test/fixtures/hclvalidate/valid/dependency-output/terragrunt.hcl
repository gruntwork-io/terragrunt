dependency "dep" {
  config_path = "./dep"
}

inputs = {
  my_input = dependency.dep.outputs.some_output
}
