terraform {
  source = "../base-module"

  # SHOULD execute.
  after_hook "after_init_from_module" {
    commands = ["init-from-module"]
    execute = ["${get_parent_terragrunt_dir()}/util.sh","ERROR MESSAGE"]
    suppress_stderr = false
  }

  # SHOULD execute.
  after_hook "after_init" {
    commands = ["init"]
    execute = ["${get_parent_terragrunt_dir()}/util.sh","ERROR MESSAGE"]
    suppress_stderr = false
  }
}
