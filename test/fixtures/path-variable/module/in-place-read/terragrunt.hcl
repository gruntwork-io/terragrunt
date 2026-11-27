terraform {
  no_dot_terragrunt_cache = true
}

inputs = {
  module      = path.download
  read_module = read_terragrunt_config("${get_terragrunt_dir()}/read.hcl").inputs.module
}
