
dependency "app1" {
  config_path = "../app1"
}

inputs = {
  input = dependency.app.outputs.result
}