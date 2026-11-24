terraform {
  no_dot_terragrunt_cache = true

  before_hook "before_plan" {
    commands = ["plan"]
    execute  = ["./scripts/record.sh", "before.out"]
  }

  after_hook "after_plan" {
    commands = ["plan"]
    execute  = ["./scripts/record.sh", "after.out"]
  }

  after_hook "after_source" {
    commands = ["init-from-module"]
    execute  = ["./scripts/record.sh", "source.out"]
  }
}
