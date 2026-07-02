terraform {
  source = "${get_terragrunt_dir()}/modules/foo"

  before_hook "shared_name_hook" {
    commands = ["apply"]
    execute  = ["${get_terragrunt_dir()}/hook.sh", "${get_terragrunt_dir()}/before.out"]
  }

  after_hook "shared_name_hook" {
    commands     = ["apply"]
    execute      = ["${get_terragrunt_dir()}/hook.sh", "${get_terragrunt_dir()}/after.out"]
    run_on_error = true
  }

  error_hook "error_hook_1" {
    commands  = ["apply"]
    on_errors = [".*"]
    execute   = ["${get_terragrunt_dir()}/hook.sh", "${get_terragrunt_dir()}/error.out"]
  }
}
