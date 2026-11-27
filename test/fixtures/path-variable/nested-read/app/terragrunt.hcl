locals {
  outer = read_terragrunt_config("../outer.hcl")
}

inputs = {
  outer_read = local.outer.locals.read_file
  inner_read = local.outer.locals.inner.locals.read_file
  inner_unit = local.outer.locals.inner.locals.read_unit
  read       = try(path.read, "not-read")
}
