terraform {
  before_hook "before_hook" {
    commands = ["plan"]
    execute  = ["touch", "${get_terragrunt_dir()}/before.out"]
  }

  after_hook "after_hook" {
    commands     = ["plan"]
    execute      = ["touch", "${get_terragrunt_dir()}/after.out"]
    run_on_error = true
  }

  error_hook "error_hook" {
    commands  = ["plan"]
    execute   = ["touch", "${get_terragrunt_dir()}/error.out"]
    on_errors = [".*"]
  }
}
