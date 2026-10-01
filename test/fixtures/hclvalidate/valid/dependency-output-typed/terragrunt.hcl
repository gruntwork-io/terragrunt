dependency "dep" {
  config_path = "./dep"
}

inputs = {
  name_set   = toset(dependency.dep.outputs.names)
  name_count = length(dependency.dep.outputs.names)
  has_names  = dependency.dep.outputs.names != null
  nested = {
    names = tolist(dependency.dep.outputs.names)
  }
}
