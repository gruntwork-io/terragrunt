locals {
  nested       = read_terragrunt_config("${dirname(try(path.read, "."))}/nested.hcl")
  stack        = try(path.stack, "no-stack")
  unit         = try(path.unit, "no-unit")
  download     = try(path.download, "no-download")
  nested_stack = local.nested.locals.stack
}
