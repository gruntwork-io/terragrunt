dependency "module_a" {
  config_path = "../module-a"
}

inputs = {
  ns = dependency.module_a.outputs.ns
}
