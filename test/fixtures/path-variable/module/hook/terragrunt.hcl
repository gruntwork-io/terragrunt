terraform {
  before_hook "module" {
    commands = ["plan"]
    execute  = ["ls", path.download]
  }

  before_hook "stack" {
    commands = ["plan"]
    execute  = ["ls", try(stack.path, "no-stack")]
  }
}

inputs = {
  module = path.download
}
