include {
  path = find_in_parent_folders("root.hcl")
}

dependency "dep" {
  config_path = "../dep"
}

inputs = {
  upstream = dependency.dep.outputs.value
}
