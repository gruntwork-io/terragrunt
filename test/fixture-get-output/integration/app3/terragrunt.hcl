locals {
  app1_path = "../app1"
  app2_path = "../app2"
}

dependency "app1" {
  config_path = local.app1_path
}

dependency "app2" {
  config_path = local.app2_path
}

inputs = {
  x = dependency.app1.outputs.x
  y = dependency.app2.outputs.y
}
