include "root" {
  path = find_in_parent_folders("root.hcl")
}

locals {
  env = read_terragrunt_config(find_in_parent_folders("env.hcl"))
}

inputs = {
  vars                 = commands.accept_vars
  locking              = commands.accept_locking
  input                = commands.accept_input
  parallelism          = commands.accept_parallelism
  indexed              = commands["accept_vars"]
  function_vars        = get_terraform_commands_that_need_vars()
  function_locking     = get_terraform_commands_that_need_locking()
  function_input       = get_terraform_commands_that_need_input()
  function_parallelism = get_terraform_commands_that_need_parallelism()
  env_input            = local.env.locals.input
  plan_accepts_vars    = contains(commands.accept_vars, "plan")
  not_defined          = try(commands.accept_nothing, "not-defined")
}
