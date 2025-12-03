dependency "module" {
  config_path = "./module"
}

inputs = {
  module_value = dependency.module.outputs.value
}

