dependency "dep" {
  config_path  = "../dep"
  skip_outputs = true
}

inputs = { from_attr = dependency.dep.outputs.value }

variables {
  inputs = { from_block = dependency.dep.outputs.value }
}
