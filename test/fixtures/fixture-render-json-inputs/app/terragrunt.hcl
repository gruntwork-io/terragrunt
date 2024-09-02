
dependency "dep" {
  config_path = "../dependency"

}

inputs = {
  static_value = "static_value"
  value = dependency.dep.outputs.value
  not_existing_value = dependency.dep.outputs.not_existing_value
}
