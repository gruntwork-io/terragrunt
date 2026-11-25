terraform {
  before_hook "module" {
    commands = ["plan"]
    execute  = ["ls", path.download]
  }

  before_hook "stack" {
    commands = ["plan"]
    execute  = ["ls", try(path.stack, "no-stack")]
  }
}

inputs = {
  module = path.download
}
