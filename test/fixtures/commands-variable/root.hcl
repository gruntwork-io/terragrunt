locals {
  locking = commands.accept_locking
}

terraform {
  extra_arguments "lock" {
    commands  = local.locking
    arguments = ["-lock-timeout=1m"]
  }

  before_hook "vars" {
    commands = commands.accept_vars
    execute  = ["touch", "${get_terragrunt_dir()}/hook.out"]
  }
}

inputs = {
  root_locking = local.locking
}
