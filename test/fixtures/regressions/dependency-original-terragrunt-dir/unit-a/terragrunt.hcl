dependency "unit_b" {
  config_path  = "../unit-b"
  skip_outputs = true
  mock_outputs = {
    app_name = "mock"
  }
}

inputs = {
  dep_name = dependency.unit_b.outputs.app_name
}
