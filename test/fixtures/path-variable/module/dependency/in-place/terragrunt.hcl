terraform {
  no_dot_terragrunt_cache = dependency.local.outputs.in_place
}

dependency "local" {
  config_path = "../../local"
}

inputs = {
  module = path.download
}
