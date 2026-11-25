locals {
  nested       = read_terragrunt_config("${dirname(try(path.read, "."))}/nested.hcl")
  stack        = try(path.stack, "no-stack")
  unit         = try(path.unit, "no-unit")
  nested_stack = local.nested.locals.stack
}

inputs = {
  download = try(path.download, "no-download")
}
