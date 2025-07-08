dependency "unit-a" {
  config_path = "../unit-a"
}

input = {
  data = dependency.unit-a.outputs.data
}