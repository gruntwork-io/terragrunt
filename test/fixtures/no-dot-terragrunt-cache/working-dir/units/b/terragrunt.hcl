terraform {
  no_dot_terragrunt_cache = true
}

dependency "a" {
  config_path = "../a"
}

inputs = {
  value = "b-${dependency.a.outputs.value}"
}
