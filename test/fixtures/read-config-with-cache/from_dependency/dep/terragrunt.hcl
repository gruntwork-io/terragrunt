locals {
  vars = read_terragrunt_config_with_cache("vars.hcl")
}

inputs = {
  foo = local.vars.locals.foo
}
