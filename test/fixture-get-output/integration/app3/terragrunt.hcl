locals {
  app1_path = "../app1"
  app2_path = "../app2"
}

dependencies {
  paths = [local.app1_path, local.app2_path]
}

inputs = {
  x = get_output(local.app1_path, "x")
  y = get_output(local.app2_path, "y")
}
