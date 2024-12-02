terraform {
  source = "${get_terragrunt_dir()}/../../modules/b"
}

include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "local" {
  path = "local.hcl"
}

dependency "c" {
  config_path = "../c"
}

inputs = {
  foo = dependency.c.outputs.foo
}
