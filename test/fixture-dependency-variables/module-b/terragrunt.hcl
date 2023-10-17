dependency "a" {
  config_path = "../module-a"

  env_vars = {}
}

dependency_env_vars = {
  "VARIANT" = "a"
}

inputs = {
  content = dependency.a.outputs.result
}