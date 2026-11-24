terraform {
  no_dot_terragrunt_cache = true
}

inputs = {
  module = path.download
}
