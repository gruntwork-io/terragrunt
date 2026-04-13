dependency "missing" {
  config_path = "../nonexistent-module"
}

dependency "valid_dep" {
  config_path = "./dep"
}

inputs = {
  from_valid   = dependency.valid_dep.outputs.value
  from_missing = dependency.missing.outputs.value
}
