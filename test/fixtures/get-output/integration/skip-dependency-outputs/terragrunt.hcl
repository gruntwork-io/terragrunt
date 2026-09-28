dependency "app1" {
  config_path = "../app1"
}

inputs = {
  x = dependency.app1.outputs.x
}
