dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    result = "mock"
  }
}

inputs = {
  dep_value = dependency.dep.outputs.result
}
