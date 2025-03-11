
dependency "app" {
  config_path = "../app"
}

inputs = {
  input = dependency.app.outputs.result
}