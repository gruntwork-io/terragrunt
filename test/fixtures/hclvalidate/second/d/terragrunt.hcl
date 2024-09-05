dependency "c" {
  config_path = "../c"

  mock_outputs = {
    c = "mocked-c"
  }
}

inputs = {
  d = dependency.c.outputs.c
}
