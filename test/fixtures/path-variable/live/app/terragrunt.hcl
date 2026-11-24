include "root" {
  path = find_in_parent_folders("root.hcl")
}

locals {
  env    = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  shared = read_terragrunt_config(find_in_parent_folders("shared.hcl"))
}

inputs = {
  unit        = unit.path
  include     = try(path.include, "not-included")
  read        = try(path.read, "not-read")
  stack       = try(stack.path, "no-stack")
  env_read    = local.env.locals.read_file
  env_unit    = local.env.locals.read_unit
  env_include = local.env.locals.read_include
  shared_read = local.shared.locals.read_file
}
