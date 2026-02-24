terraform {

  # This hook configures Terragrunt to create an empty file called file.out
  # before execution of terragrunt
  before_hook "before_hook_1" {
    commands = ["apply", "plan"]
    execute = ["touch","${get_terragrunt_dir()}/file.out"]
    run_on_error = true
  }
}
