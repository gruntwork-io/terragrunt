
dependency "dependency" {
  config_path = "../dependency"
}

inputs = {
  input_value = dependency.dependency.outputs.result
}