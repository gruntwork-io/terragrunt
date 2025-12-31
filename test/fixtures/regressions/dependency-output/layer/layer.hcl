dependency "dep" {
  config_path = "../dep"
  mock_outputs = {
    name = "mock-name"
  }
}

inputs = {
  dep_name = dependency.dep.outputs.name
}
