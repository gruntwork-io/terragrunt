terraform {
  before_hook "module" {
    commands = ["plan"]
    execute  = ["ls", path.module]
  }

  before_hook "stack" {
    commands = ["plan"]
    execute  = ["ls", try(path.stack, "no-stack")]
  }
}

inputs = {
  module = path.module
}
