locals {
  vars = read_terragrunt_config("--terragrunt-with-cache", "vars.hcl")
}

inputs = {
  foo = local.vars.locals.foo
}
