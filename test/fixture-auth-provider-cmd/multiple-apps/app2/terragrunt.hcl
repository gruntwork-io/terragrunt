include {
  path = find_in_parent_folders()
}

dependency "app3" {
  config_path  = "../app3"
}

inputs = {
  foo-app3 = dependency.app3.outputs.foo-app3
}
