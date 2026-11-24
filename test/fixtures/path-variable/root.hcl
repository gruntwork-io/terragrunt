locals {
  included_file = path.include
}

inputs = {
  root_unit    = unit.path
  root_include = path.include
  root_local   = local.included_file
  root_read    = try(path.read, "not-read")
}
