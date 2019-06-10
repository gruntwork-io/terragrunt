terraform {
  # This hook configures Terragrunt to create an empty file called file.out
  # after execution of terragrunt
  after_hook "after_hook_1" {
    commands = ["apply", "plan"]
    execute = ["touch","file.out"]
  }
}
