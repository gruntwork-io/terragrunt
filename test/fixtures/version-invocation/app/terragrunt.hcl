
dependency "dependency" {
  config_path = "../dependency"
}

dependency "dependency-with-custom-version" {
  config_path = "../dependency-with-custom-version"
}

inputs = {
  input_value = dependency.dependency.outputs.result
}