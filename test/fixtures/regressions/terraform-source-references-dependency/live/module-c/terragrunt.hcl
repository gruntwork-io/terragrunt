terraform {
  source = "../../modules//module-c"
}

dependency "module_b" {
  config_path = "../module-b"
}

inputs = {
  ns = dependency.module_b.outputs.ns
}
