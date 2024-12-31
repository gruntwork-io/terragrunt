include {
  path = find_in_parent_folders("root.hcl")
}

dependency "app2" {
  config_path  = "../app2"
}

inputs = {
  foo-app2 = dependency.app2.outputs.foo-app2
  foo-app3 = dependency.app2.outputs.foo-app3
}
