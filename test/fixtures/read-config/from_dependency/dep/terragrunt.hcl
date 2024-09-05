locals {
  vars = read_terragrunt_config("vars.hcl")
}

inputs = {
  foo = local.vars.locals.foo
}
