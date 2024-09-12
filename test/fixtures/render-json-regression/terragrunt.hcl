terraform {
  source = "./foo"
}

locals {
  foo = "bar"
}

include "root" {
  path   = "./bar/terragrunt.hcl"
  expose = true
}

dependency "baz" {
  config_path = "./baz"
}

generate "provider" {
  path = "provider.tf"
  if_exists = "overwrite"
  contents = "# This is just a test"
}

inputs = {
  foo = "bar"
  baz = "blah"
  another = dependency.baz.outputs.baz
}
